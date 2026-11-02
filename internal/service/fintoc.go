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
	"github.com/flowpay/flowpay-payments/internal/gateway/fintoc"
	"github.com/flowpay/flowpay-payments/internal/model"
	"github.com/flowpay/flowpay-payments/internal/secretbox"
)

func (s *PaymentsService) companyFintocSecrets(ctx context.Context, companyID int64) (secretKey, webhookSecret string, err error) {
	sealedKey, sealedHook, ok, err := s.Repo.CompanyFintocSealed(ctx, companyID)
	if err != nil {
		return "", "", err
	}
	if !ok {
		return "", "", domain.ErrFintocNotOffered
	}
	if len(s.PaymentKey) != 32 {
		return "", "", domain.ErrFintocCredential
	}
	secretKey, err = secretbox.Open(s.PaymentKey, sealedKey, webpayAAD(companyID, "fintoc_secret_key"))
	if err != nil || strings.TrimSpace(secretKey) == "" {
		log.Printf("[FlowPay Fintoc] empresa %d: secret key no usable", companyID)
		return "", "", domain.ErrFintocCredential
	}
	if strings.TrimSpace(sealedHook) != "" {
		webhookSecret, err = secretbox.Open(s.PaymentKey, sealedHook, webpayAAD(companyID, "fintoc_webhook_secret"))
		if err != nil {
			log.Printf("[FlowPay Fintoc] empresa %d: secreto de webhook no usable", companyID)
			return "", "", domain.ErrFintocCredential
		}
	}
	return secretKey, webhookSecret, nil
}

func (s *PaymentsService) StartFintocCheckout(ctx context.Context, portalToken string, chargeRefs []string) (*WebpayCheckoutResult, error) {
	if s.Webpay == nil || strings.TrimSpace(s.Webpay.PublicBaseURL) == "" {
		return nil, errors.New("FLOWPAY_PAYMENTS_PUBLIC_BASE_URL no configurada")
	}
	portalToken = strings.TrimSpace(portalToken)
	pt, charges, amountCLP, err := s.resolvePortalCheckout(ctx, portalToken, chargeRefs)
	if err != nil {
		return nil, err
	}
	if err := s.requireTransfer(ctx, pt.CompanyID, fintocGateway); err != nil {
		return nil, err
	}
	secretKey, _, err := s.companyFintocSecrets(ctx, pt.CompanyID)
	if err != nil {
		return nil, err
	}
	client := fintoc.NewClient(secretKey)
	if !client.Enabled() {
		return nil, domain.ErrFintocNotOffered
	}

	buyOrder, err := newBuyOrder(pt.CompanyID)
	if err != nil {
		return nil, err
	}
	publicBase := strings.TrimSuffix(s.Webpay.PublicBaseURL, "/")
	returnURL := publicBase + "/api/public/fintoc/return/" + url.PathEscape(portalToken) + "?orden=" + url.QueryEscape(buyOrder)
	cancelURL := returnURL + "&cancel=1"

	row := &model.PaymentTransaction{
		CompanyID:   pt.CompanyID,
		ClientID:    pt.ClientID,
		Gateway:     fintocGateway,
		Environment: fintocGateway,
		BuyOrder:    buyOrder,
		SessionID:   "fi-" + buyOrder,
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

	created, rawCreate, err := client.CreateSession(ctx, fintoc.CreateSessionRequest{
		Amount:             amountCLP,
		Currency:           "CLP",
		SuccessURL:         returnURL,
		CancelURL:          cancelURL,
		PaymentMethodTypes: []string{"bank_transfer"},
		Metadata:           map[string]string{"buy_order": buyOrder},
	})
	if err != nil {
		_ = s.Repo.FailPaymentTransaction(ctx, txID, "create_error", rawCreate)
		s.noteGateway(ctx, audit.ActionPaymentFailed, audit.OutcomeFailure, pt.CompanyID, txID, "payer", "No se pudo iniciar el pago con Fintoc", "fintoc_create", audit.Cause(err), map[string]any{"gateway": fintocGateway, "buy_order": buyOrder, "amount": amountCLP})
		return nil, fmt.Errorf("fintoc create: %w", err)
	}
	if err := s.Repo.UpdatePaymentTransactionWebpayCreate(ctx, txID, created.ID, created.RedirectURL, "redirected", rawCreate); err != nil {
		return nil, err
	}
	s.record(ctx, audit.Event{
		Domain:     audit.DomainPayment,
		Action:     audit.ActionCheckoutStarted,
		ActorRole:  "payer",
		CompanyID:  pt.CompanyID,
		TargetType: audit.TargetPayment,
		TargetID:   audit.ID(txID),
		Summary:    "Se inició un pago con Fintoc",
		Meta:       map[string]any{"gateway": fintocGateway, "buy_order": buyOrder, "amount": amountCLP, "charge_ids": chargeIDs(charges)},
	})
	return &WebpayCheckoutResult{
		RedirectURL: created.RedirectURL,
		BuyOrder:    buyOrder,
	}, nil
}

// ReadFintocStatus solo lee la transacción. Quien marca los cobros es el webhook.
func (s *PaymentsService) ReadFintocStatus(ctx context.Context, portalToken, buyOrder string) (*WebpayCommitResult, error) {
	row, err := s.fintocTransaction(ctx, portalToken, buyOrder, "")
	if err != nil {
		return nil, err
	}
	return fintocResultFromRow(row), nil
}

// NotifyFintoc marca el cobro si la firma es válida, el monto coincide y todavía no estaba pagado.
func (s *PaymentsService) NotifyFintoc(ctx context.Context, signature string, raw []byte, now time.Time) error {
	decision, err := fintoc.Interpret(raw)
	if err != nil || decision.Action == "ignore" && decision.SessionID == "" && decision.BuyOrder == "" {
		return nil
	}
	row, err := s.fintocTransaction(ctx, "", decision.BuyOrder, decision.SessionID)
	if err != nil {
		if errors.Is(err, domain.ErrWebpayTxNotFound) {
			log.Printf("[FlowPay Fintoc] aviso sin cobro session=%s orden=%s", decision.SessionID, decision.BuyOrder)
			return nil
		}
		return err
	}
	_, webhookSecret, err := s.companyFintocSecrets(ctx, row.CompanyID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(webhookSecret) == "" || !fintoc.VerifySignature(signature, webhookSecret, raw, now) {
		s.noteGateway(ctx, audit.ActionWebhookRejected, audit.OutcomeDenied, row.CompanyID, row.ID, "webhook", "Webhook de Fintoc rechazado", "fintoc_signature", "la firma no es válida", map[string]any{"gateway": fintocGateway})
		return domain.ErrFintocSignature
	}
	if decision.Action == "ignore" {
		return nil
	}
	s.record(ctx, audit.Event{
		Domain:     audit.DomainPayment,
		Action:     audit.ActionWebhookAccepted,
		ActorRole:  "webhook",
		CompanyID:  row.CompanyID,
		TargetType: audit.TargetPayment,
		TargetID:   audit.ID(row.ID),
		Summary:    "Webhook de Fintoc aceptado",
		Meta:       map[string]any{"gateway": fintocGateway, "buy_order": row.BuyOrder},
	})
	return s.applyFintocDecision(ctx, row, decision, raw)
}

func (s *PaymentsService) fintocTransaction(ctx context.Context, portalToken, buyOrder, sessionID string) (*model.PaymentTransaction, error) {
	buyOrder = strings.TrimSpace(buyOrder)
	sessionID = strings.TrimSpace(sessionID)
	if buyOrder == "" && sessionID == "" {
		return nil, domain.ErrWebpayTxNotFound
	}
	var row *model.PaymentTransaction
	var err error
	if sessionID != "" {
		row, err = s.Repo.GetPaymentTransactionByWebpayToken(ctx, sessionID)
		if errors.Is(err, sql.ErrNoRows) && buyOrder != "" {
			row, err = s.Repo.GetPaymentTransactionByBuyOrder(ctx, buyOrder)
		}
	} else {
		row, err = s.Repo.GetPaymentTransactionByBuyOrder(ctx, buyOrder)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrWebpayTxNotFound
		}
		return nil, err
	}
	if row.Gateway != fintocGateway {
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

func (s *PaymentsService) applyFintocDecision(ctx context.Context, row *model.PaymentTransaction, decision fintoc.Decision, raw []byte) error {
	if row.Status == "authorized" {
		return nil
	}
	if decision.Action == "mismatch" {
		reason := strings.TrimSpace(decision.Reason)
		if reason == "" {
			reason = "el aviso no coincide con el cobro"
		}
		s.noteFintocMismatch(ctx, row, reason)
		return domain.ErrFintocMismatch
	}
	if decision.Action == "fail" {
		_ = s.Repo.FailPaymentTransaction(ctx, row.ID, "failed", raw)
		s.noteGateway(ctx, audit.ActionPaymentFailed, audit.OutcomeFailure, row.CompanyID, row.ID, "webhook", "La transferencia Fintoc no se completó", "fintoc_failed", gatewayStatusPhrase(decision.Status), map[string]any{"gateway": fintocGateway, "buy_order": row.BuyOrder, "amount": row.Amount, "status": decision.Status})
		return nil
	}
	if decision.Action != "pay" {
		return nil
	}
	if decision.SessionID != "" && row.WebpayToken.Valid && decision.SessionID != row.WebpayToken.String {
		s.noteFintocMismatch(ctx, row, "la sesión no coincide")
		return domain.ErrFintocMismatch
	}
	if decision.BuyOrder != "" && decision.BuyOrder != row.BuyOrder {
		s.noteFintocMismatch(ctx, row, "la orden no coincide")
		return domain.ErrFintocMismatch
	}
	if !strings.EqualFold(strings.TrimSpace(decision.Currency), "CLP") || decision.Amount != int64(math.Round(row.Amount)) {
		s.noteFintocMismatch(ctx, row, "el monto o la moneda no coinciden")
		return domain.ErrFintocMismatch
	}
	code := strings.TrimSpace(decision.PaymentID)
	if code == "" {
		code = strings.TrimSpace(decision.SessionID)
	}
	if len(code) > 32 {
		code = code[:32]
	}
	claimed, err := s.Repo.AuthorizePaymentTransaction(ctx, row.ID, &model.CommitUpdate{
		AuthorizationCode: code,
		PaymentTypeCode:   fintocGateway,
		TransbankStatus:   "bank_transfer",
		RawCommit:         raw,
	})
	if err != nil {
		s.noteGateway(ctx, audit.ActionPaymentFailed, audit.OutcomeFailure, row.CompanyID, row.ID, "webhook", "No se pudo registrar la transferencia Fintoc", "fintoc_store", "no se pudo guardar la confirmación", map[string]any{"gateway": fintocGateway, "buy_order": row.BuyOrder, "amount": row.Amount})
		return err
	}
	if !claimed {
		return nil
	}
	charges, err := s.Repo.ListPaymentTransactionCharges(ctx, row.ID)
	if err != nil {
		s.noteGateway(ctx, audit.ActionPaymentFailed, audit.OutcomeFailure, row.CompanyID, row.ID, "webhook", "No se pudo registrar la transferencia Fintoc", "fintoc_store", "no se pudieron leer los cobros del pago", map[string]any{"gateway": fintocGateway, "buy_order": row.BuyOrder, "amount": row.Amount})
		return err
	}
	ref := strings.TrimSpace(decision.PaymentID)
	if ref == "" {
		ref = strings.TrimSpace(decision.SessionID)
	}
	for _, link := range charges {
		if err := s.Repo.MarkChargePaidGateway(ctx, link.ChargeID, link.Amount, row.ID, fintocGateway, ref); err != nil {
			s.noteGateway(ctx, audit.ActionPaymentFailed, audit.OutcomeFailure, row.CompanyID, row.ID, "webhook", "No se pudo registrar la transferencia Fintoc", "fintoc_store", "no se pudo marcar el cobro como pagado", map[string]any{"gateway": fintocGateway, "buy_order": row.BuyOrder, "amount": row.Amount})
			return err
		}
	}
	if row.PaymentTokenID.Valid {
		_ = s.Repo.MarkPaymentTokenPaidByID(ctx, row.PaymentTokenID.Int64)
	}
	s.record(ctx, audit.Event{
		Domain:     audit.DomainPayment,
		Action:     audit.ActionPaymentCommitted,
		ActorRole:  "webhook",
		CompanyID:  row.CompanyID,
		TargetType: audit.TargetPayment,
		TargetID:   audit.ID(row.ID),
		Summary:    "Transferencia Fintoc confirmada",
		Meta:       map[string]any{"gateway": fintocGateway, "buy_order": row.BuyOrder, "amount": row.Amount},
	})
	return nil
}

func (s *PaymentsService) noteFintocMismatch(ctx context.Context, row *model.PaymentTransaction, reason string) {
	if row == nil || !audit.Once("fintoc-mismatch:"+audit.ID(row.ID)+":"+reason, 10*time.Minute) {
		return
	}
	s.noteGateway(ctx, audit.ActionPaymentFailed, audit.OutcomeDenied, row.CompanyID, row.ID, "webhook", "El aviso de Fintoc no coincide con el cobro", "fintoc_mismatch", reason, map[string]any{"gateway": fintocGateway, "buy_order": row.BuyOrder, "amount": row.Amount})
}

func fintocResultFromRow(row *model.PaymentTransaction) *WebpayCommitResult {
	out := &WebpayCommitResult{
		BuyOrder: row.BuyOrder,
		Amount:   row.Amount,
		Method:   fintocGateway,
	}
	switch row.Status {
	case "authorized":
		out.Status = "authorized"
		out.Message = "Transferencia confirmada"
		if row.AuthorizationCode.Valid {
			out.AuthorizationCode = row.AuthorizationCode.String
		}
	case "failed":
		out.Status = "failed"
		out.Message = "La transferencia no se completó"
	default:
		out.Status = "pending"
		out.Message = "Fintoc está confirmando la transferencia"
	}
	return out
}
