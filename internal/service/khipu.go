package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/flowpay/flowpay-payments/internal/audit"
	"github.com/flowpay/flowpay-payments/internal/domain"
	"github.com/flowpay/flowpay-payments/internal/gateway/khipu"
	"github.com/flowpay/flowpay-payments/internal/model"
	"github.com/flowpay/flowpay-payments/internal/secretbox"
)

func (s *PaymentsService) companyKhipuClient(ctx context.Context, companyID int64) (*khipu.Client, error) {
	sealed, ok, err := s.Repo.CompanyKhipuSealed(ctx, companyID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrKhipuNotOffered
	}
	client, err := khipuClientFromSecret(s.PaymentKey, companyID, sealed)
	if errors.Is(err, domain.ErrKhipuCredential) {
		log.Printf("[FlowPay Khipu] empresa %d: credencial no usable", companyID)
	}
	return client, err
}

func khipuClientFromSecret(key []byte, companyID int64, sealed string) (*khipu.Client, error) {
	if len(key) != 32 {
		return nil, domain.ErrKhipuCredential
	}
	secret, err := secretbox.Open(key, sealed, webpayAAD(companyID, "khipu_api_key"))
	if err != nil {
		return nil, domain.ErrKhipuCredential
	}
	client := khipu.NewClient(secret)
	if !client.Enabled() {
		return nil, domain.ErrKhipuNotOffered
	}
	return client, nil
}

func (s *PaymentsService) StartKhipuCheckout(ctx context.Context, portalToken string, chargeRefs []string) (*WebpayCheckoutResult, error) {
	if s.Webpay == nil || strings.TrimSpace(s.Webpay.PublicBaseURL) == "" {
		return nil, errors.New("FLOWPAY_PAYMENTS_PUBLIC_BASE_URL no configurada")
	}
	portalToken = strings.TrimSpace(portalToken)
	pt, charges, amountCLP, err := s.resolvePortalCheckout(ctx, portalToken, chargeRefs)
	if err != nil {
		return nil, err
	}
	if err := s.requireTransfer(ctx, pt.CompanyID, khipuGateway); err != nil {
		return nil, err
	}
	client, err := s.companyKhipuClient(ctx, pt.CompanyID)
	if err != nil {
		return nil, err
	}

	buyOrder, err := newBuyOrder(pt.CompanyID)
	if err != nil {
		return nil, err
	}
	publicBase := strings.TrimSuffix(s.Webpay.PublicBaseURL, "/")
	returnURL := publicBase + "/api/public/khipu/return/" + url.PathEscape(portalToken) + "?orden=" + url.QueryEscape(buyOrder)
	cancelURL := returnURL + "&cancel=1"
	notifyURL := khipuNotifyURL(publicBase)
	if notifyURL == "" {
		log.Printf("[FlowPay Khipu] sin aviso a Khipu: %s no es una URL HTTPS pública. El cobro se confirma cuando el pagador vuelve.", publicBase)
	}

	row := &model.PaymentTransaction{
		CompanyID:   pt.CompanyID,
		ClientID:    pt.ClientID,
		Gateway:     khipuGateway,
		Environment: khipuGateway,
		BuyOrder:    buyOrder,
		SessionID:   "kh-" + buyOrder,
		Amount:      float64(amountCLP),
		Currency:    "CLP",
		Status:      "created",
		ReturnURL:   returnURL,
	}
	row.PaymentTokenID = sql.NullInt64{Int64: pt.ID, Valid: true}

	txID, err := s.Repo.InsertPaymentTransaction(ctx, row)
	if err != nil {
		return nil, err
	}
	var junction []model.PaymentTransactionCharge
	for _, ch := range charges {
		junction = append(junction, model.PaymentTransactionCharge{
			TransactionID: txID,
			ChargeID:      ch.ID,
			Amount:        ch.Amount,
		})
	}
	if err := s.Repo.InsertPaymentTransactionCharges(ctx, txID, junction); err != nil {
		return nil, err
	}

	subject := "Pago " + buyOrder
	createReq := khipu.CreateRequest{
		Amount:        float64(amountCLP),
		Currency:      "CLP",
		Subject:       subject,
		TransactionID: buyOrder,
		ReturnURL:     returnURL,
		CancelURL:     cancelURL,
	}
	if notifyURL != "" {
		createReq.NotifyURL = notifyURL
		createReq.NotifyAPIVersion = "3.0"
	}
	created, rawCreate, err := client.Create(ctx, createReq)
	if err != nil {
		_ = s.Repo.FailPaymentTransaction(ctx, txID, "create_error", rawCreate)
		s.noteGateway(ctx, audit.ActionPaymentFailed, audit.OutcomeFailure, pt.CompanyID, txID, "payer", "No se pudo iniciar el pago con Khipu", "khipu_create", audit.Cause(err), map[string]any{"gateway": khipuGateway, "buy_order": buyOrder, "amount": amountCLP})
		return nil, fmt.Errorf("khipu create: %w", err)
	}
	if err := s.Repo.UpdatePaymentTransactionWebpayCreate(ctx, txID, created.PaymentID, created.PaymentURL, "redirected", rawCreate); err != nil {
		return nil, err
	}
	s.record(ctx, audit.Event{
		Domain:     audit.DomainPayment,
		Action:     audit.ActionCheckoutStarted,
		ActorRole:  "payer",
		CompanyID:  pt.CompanyID,
		TargetType: audit.TargetPayment,
		TargetID:   audit.ID(txID),
		Summary:    "Se inició un pago con Khipu",
		Meta:       map[string]any{"gateway": khipuGateway, "buy_order": buyOrder, "amount": amountCLP, "charge_ids": chargeIDs(charges)},
	})
	return &WebpayCheckoutResult{
		RedirectURL: created.PaymentURL,
		BuyOrder:    buyOrder,
	}, nil
}

// SyncKhipuReturn consulta Khipu al volver el pagador. Si todavía no está conciliado, queda pendiente.
func (s *PaymentsService) SyncKhipuReturn(ctx context.Context, portalToken, buyOrder string) (*WebpayCommitResult, error) {
	row, err := s.khipuTransaction(ctx, portalToken, buyOrder, "")
	if err != nil {
		return nil, err
	}
	if row.Status == "authorized" {
		out := s.commitResultFromRow(row)
		out.Method = khipuGateway
		out.Message = "Transferencia confirmada"
		return out, nil
	}
	if !row.WebpayToken.Valid || strings.TrimSpace(row.WebpayToken.String) == "" {
		return nil, domain.ErrWebpayTxNotFound
	}
	client, err := s.companyKhipuClient(ctx, row.CompanyID)
	if err != nil {
		return nil, err
	}
	payment, raw, err := client.Get(ctx, row.WebpayToken.String)
	if err != nil {
		return nil, err
	}
	out, err := s.applyKhipuPayment(ctx, portalToken, row, payment, raw)
	if errors.Is(err, domain.ErrKhipuPending) {
		return out, nil
	}
	return out, err
}

func (s *PaymentsService) noteUnknownKhipu(ctx context.Context, paymentID string) {
	stamped := audit.Finish(ctx, audit.Event{})
	ip := stamped.IP
	if ip == "" {
		ip = "unknown"
	}
	if !audit.Once("khipu-unknown:"+ip, 10*time.Minute) {
		return
	}
	if len(paymentID) > 40 {
		paymentID = paymentID[:40]
	}
	s.noteGateway(ctx, audit.ActionWebhookRejected, audit.OutcomeDenied, 0, 0, "webhook", "Aviso de Khipu sin un pago nuestro", "khipu_unknown", "no corresponde a un pago de GeldFlus", map[string]any{"gateway": khipuGateway, "payment_id": paymentID})
}

// NotifyKhipu confirma un aviso de conciliación. ErrKhipuPending pide que Khipu reintente.
func (s *PaymentsService) NotifyKhipu(ctx context.Context, paymentID string) error {
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return nil
	}
	row, err := s.Repo.GetPaymentTransactionByWebpayToken(ctx, paymentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.noteUnknownKhipu(ctx, paymentID)
			return domain.ErrWebpayTxNotFound
		}
		return err
	}
	if row.Gateway != khipuGateway {
		s.noteUnknownKhipu(ctx, paymentID)
		return domain.ErrWebpayTxNotFound
	}
	if row.Status == "authorized" {
		return nil
	}
	s.record(ctx, audit.Event{
		Domain:     audit.DomainPayment,
		Action:     audit.ActionWebhookAccepted,
		ActorRole:  "webhook",
		CompanyID:  row.CompanyID,
		TargetType: audit.TargetPayment,
		TargetID:   audit.ID(row.ID),
		Summary:    "Webhook de Khipu aceptado",
		Meta:       map[string]any{"gateway": khipuGateway, "buy_order": row.BuyOrder},
	})
	client, err := s.companyKhipuClient(ctx, row.CompanyID)
	if err != nil {
		return err
	}
	payment, raw, err := client.Get(ctx, paymentID)
	if err != nil {
		return err
	}
	_, err = s.applyKhipuPayment(ctx, "", row, payment, raw)
	return err
}

func (s *PaymentsService) khipuTransaction(ctx context.Context, portalToken, buyOrder, paymentID string) (*model.PaymentTransaction, error) {
	buyOrder = strings.TrimSpace(buyOrder)
	if buyOrder == "" && paymentID == "" {
		return nil, domain.ErrWebpayTxNotFound
	}
	var row *model.PaymentTransaction
	var err error
	if buyOrder != "" {
		row, err = s.Repo.GetPaymentTransactionByBuyOrder(ctx, buyOrder)
	} else {
		row, err = s.Repo.GetPaymentTransactionByWebpayToken(ctx, paymentID)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrWebpayTxNotFound
		}
		return nil, err
	}
	if row.Gateway != khipuGateway {
		return nil, domain.ErrWebpayTxNotFound
	}
	if strings.TrimSpace(portalToken) == "" {
		return row, nil
	}
	pt, err := s.Repo.GetPaymentTokenByValue(ctx, strings.TrimSpace(portalToken))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrPaymentTokenNotFound
		}
		return nil, err
	}
	if !row.PaymentTokenID.Valid || row.PaymentTokenID.Int64 != pt.ID || row.CompanyID != pt.CompanyID || row.ClientID != pt.ClientID {
		return nil, domain.ErrWebpayTxNotFound
	}
	return row, nil
}

func (s *PaymentsService) applyKhipuPayment(ctx context.Context, portalToken string, row *model.PaymentTransaction, payment *khipu.Payment, raw []byte) (*WebpayCommitResult, error) {
	if row.Status == "authorized" {
		out := s.commitResultFromRow(row)
		out.Method = khipuGateway
		out.Message = "Transferencia confirmada"
		return out, nil
	}
	if payment == nil || !payment.Reconciled() {
		if payment != nil && payment.Terminal() {
			_ = s.Repo.FailPaymentTransaction(ctx, row.ID, clipStatus(payment.Status), raw)
			s.noteGateway(ctx, audit.ActionPaymentFailed, audit.OutcomeFailure, row.CompanyID, row.ID, "payer", "La transferencia Khipu no se completó", "khipu_failed", gatewayStatusPhrase(payment.Status), map[string]any{"gateway": khipuGateway, "buy_order": row.BuyOrder, "amount": row.Amount, "status": clipStatus(payment.Status)})
			return &WebpayCommitResult{
				Status:   "failed",
				BuyOrder: row.BuyOrder,
				Amount:   row.Amount,
				Method:   khipuGateway,
				Message:  "La transferencia no se completó",
			}, nil
		}
		return &WebpayCommitResult{
			Status:   "pending",
			BuyOrder: row.BuyOrder,
			Amount:   row.Amount,
			Method:   khipuGateway,
			Message:  "Khipu está confirmando la transferencia",
		}, domain.ErrKhipuPending
	}
	if row.WebpayToken.Valid && payment.PaymentID != "" && payment.PaymentID != row.WebpayToken.String {
		s.noteKhipuMismatch(ctx, row, "el identificador no coincide")
		return nil, domain.ErrKhipuMismatch
	}
	if payment.TransactionID != "" && payment.TransactionID != row.BuyOrder {
		s.noteKhipuMismatch(ctx, row, "la orden no coincide")
		return nil, domain.ErrKhipuMismatch
	}
	if !strings.EqualFold(strings.TrimSpace(payment.Currency), "CLP") || int64(math.Round(float64(payment.Amount))) != int64(math.Round(row.Amount)) {
		s.noteKhipuMismatch(ctx, row, "el monto o la moneda no coinciden")
		return nil, domain.ErrKhipuMismatch
	}

	code := strings.TrimSpace(payment.AuthorizerOperationCode)
	if code == "" {
		code = strings.TrimSpace(payment.PaymentID)
	}
	if len(code) > 32 {
		code = code[:32]
	}
	claimed, err := s.Repo.AuthorizePaymentTransaction(ctx, row.ID, &model.CommitUpdate{
		AuthorizationCode: code,
		PaymentTypeCode:   "khipu",
		TransbankStatus:   clipStatus(payment.PaymentMethod),
		RawCommit:         raw,
	})
	if err != nil {
		s.noteGateway(ctx, audit.ActionPaymentFailed, audit.OutcomeFailure, row.CompanyID, row.ID, "payer", "No se pudo registrar la transferencia Khipu", "khipu_store", "no se pudo guardar la confirmación", map[string]any{"gateway": khipuGateway, "buy_order": row.BuyOrder, "amount": row.Amount})
		return nil, err
	}
	if claimed {
		charges, err := s.Repo.ListPaymentTransactionCharges(ctx, row.ID)
		if err != nil {
			s.noteGateway(ctx, audit.ActionPaymentFailed, audit.OutcomeFailure, row.CompanyID, row.ID, "payer", "No se pudo registrar la transferencia Khipu", "khipu_store", "no se pudieron leer los cobros del pago", map[string]any{"gateway": khipuGateway, "buy_order": row.BuyOrder, "amount": row.Amount})
			return nil, err
		}
		ref := strings.TrimSpace(payment.PaymentID)
		for _, link := range charges {
			if err := s.Repo.MarkChargePaidGateway(ctx, link.ChargeID, link.Amount, row.ID, khipuGateway, ref); err != nil {
				s.noteGateway(ctx, audit.ActionPaymentFailed, audit.OutcomeFailure, row.CompanyID, row.ID, "payer", "No se pudo registrar la transferencia Khipu", "khipu_store", "no se pudo marcar el cobro como pagado", map[string]any{"gateway": khipuGateway, "buy_order": row.BuyOrder, "amount": row.Amount})
				return nil, err
			}
		}
		if row.PaymentTokenID.Valid {
			_ = s.Repo.MarkPaymentTokenPaidByID(ctx, row.PaymentTokenID.Int64)
		} else if strings.TrimSpace(portalToken) != "" {
			_ = s.Repo.MarkPaymentTokenPaid(ctx, portalToken)
		}
		s.record(ctx, audit.Event{
			Domain:     audit.DomainPayment,
			Action:     audit.ActionPaymentCommitted,
			ActorRole:  "payer",
			CompanyID:  row.CompanyID,
			TargetType: audit.TargetPayment,
			TargetID:   audit.ID(row.ID),
			Summary:    "Transferencia Khipu confirmada",
			Meta:       map[string]any{"gateway": khipuGateway, "buy_order": row.BuyOrder, "amount": row.Amount},
		})
	}
	row.Status = "authorized"
	if row.AuthorizationCode.String == "" {
		row.AuthorizationCode = sql.NullString{String: code, Valid: code != ""}
	}
	out := s.commitResultFromRow(row)
	out.Method = khipuGateway
	out.AuthorizationCode = code
	out.Message = "Transferencia confirmada"
	return out, nil
}

// khipuNotifyURL es el webhook solo si Khipu puede llamarlo. localhost y HTTP no sirven.
func khipuNotifyURL(publicBase string) string {
	parsed, err := url.Parse(strings.TrimSpace(publicBase))
	if err != nil || parsed.Host == "" || !strings.EqualFold(parsed.Scheme, "https") {
		return ""
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "localhost", "127.0.0.1", "::1":
		return ""
	}
	return strings.TrimSuffix(publicBase, "/") + "/api/public/khipu/notify"
}

func (s *PaymentsService) noteKhipuMismatch(ctx context.Context, row *model.PaymentTransaction, reason string) {
	if row == nil || !audit.Once("khipu-mismatch:"+audit.ID(row.ID)+":"+reason, 10*time.Minute) {
		return
	}
	s.noteGateway(ctx, audit.ActionPaymentFailed, audit.OutcomeDenied, row.CompanyID, row.ID, "webhook", "El aviso de Khipu no coincide con el cobro", "khipu_mismatch", reason, map[string]any{"gateway": khipuGateway, "buy_order": row.BuyOrder, "amount": row.Amount})
}

func clipStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "reconciled"
	}
	if len(status) > 32 {
		return status[:32]
	}
	return status
}
