package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/flowpay/flowpay-payments/internal/domain"
	"github.com/flowpay/flowpay-payments/internal/gateway/transbank"
	"github.com/flowpay/flowpay-payments/internal/model"
)

type WebpayCheckoutResult struct {
	RedirectURL string `json:"redirect_url"`
	BuyOrder    string `json:"buy_order,omitempty"`
}

type WebpayCommitResult struct {
	Status            string  `json:"status"`
	BuyOrder          string  `json:"buy_order,omitempty"`
	AuthorizationCode string  `json:"authorization_code,omitempty"`
	Amount            float64 `json:"amount,omitempty"`
	Message           string  `json:"message,omitempty"`
}

func (s *PaymentsService) StartWebpayCheckout(ctx context.Context, portalToken string, chargeRefs []string) (*WebpayCheckoutResult, error) {
	if !s.webpayEnabled() {
		return nil, domain.ErrWebpayNotConfigured
	}
	if strings.TrimSpace(s.Webpay.PublicBaseURL) == "" {
		return nil, errors.New("FLOWPAY_PAYMENTS_PUBLIC_BASE_URL no configurada")
	}
	portalToken = strings.TrimSpace(portalToken)
	pt, charges, amountCLP, err := s.resolvePortalCheckout(ctx, portalToken, chargeRefs)
	if err != nil {
		return nil, err
	}

	buyOrder, err := newBuyOrder(pt.CompanyID)
	if err != nil {
		return nil, err
	}
	sessionID := fmt.Sprintf("fp%d-%d", pt.ID, time.Now().UnixNano())
	if len(sessionID) > 61 {
		sessionID = sessionID[:61]
	}

	returnURL := strings.TrimSuffix(s.Webpay.PublicBaseURL, "/") +
		"/api/public/webpay/return/" + url.PathEscape(portalToken)

	row := &model.PaymentTransaction{
		CompanyID:   pt.CompanyID,
		ClientID:    pt.ClientID,
		Gateway:     "transbank_webpay_plus",
		Environment: strings.ToLower(strings.TrimSpace(s.Webpay.Environment)),
		BuyOrder:    buyOrder,
		SessionID:   sessionID,
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

	createResp, rawCreate, err := s.Webpay.Transbank.Create(ctx, transbank.CreateRequest{
		BuyOrder:  buyOrder,
		SessionID: sessionID,
		Amount:    amountCLP,
		ReturnURL: returnURL,
	})
	if err != nil {
		_ = s.Repo.FailPaymentTransaction(ctx, txID, "create_error", rawCreate)
		return nil, fmt.Errorf("transbank create: %w", err)
	}

	if err := s.Repo.UpdatePaymentTransactionWebpayCreate(ctx, txID, createResp.Token, createResp.URL, "redirected", rawCreate); err != nil {
		return nil, err
	}

	bridgeURL := strings.TrimSuffix(s.Webpay.PublicBaseURL, "/") +
		"/api/public/webpay/bridge/" + fmt.Sprintf("%d", txID)

	return &WebpayCheckoutResult{
		RedirectURL: bridgeURL,
		BuyOrder:    buyOrder,
	}, nil
}

func (s *PaymentsService) WebpayBridge(ctx context.Context, transactionID int64) (actionURL, token string, err error) {
	row, err := s.Repo.GetPaymentTransactionByID(ctx, transactionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", domain.ErrWebpayTxNotFound
		}
		return "", "", fmt.Errorf("leer transacción: %w", err)
	}
	if !row.WebpayToken.Valid || row.WebpayToken.String == "" {
		return "", "", errors.New("transacción sin token webpay")
	}
	actionURL = ""
	if row.WebpayRedirectURL.Valid {
		actionURL = strings.TrimSpace(row.WebpayRedirectURL.String)
	}
	if actionURL == "" && len(row.RawCreate) > 0 {
		var raw map[string]any
		if json.Unmarshal(row.RawCreate, &raw) == nil {
			if u, ok := raw["url"].(string); ok {
				actionURL = u
			}
		}
	}
	if actionURL == "" {
		return "", "", errors.New("url de webpay no disponible")
	}
	return actionURL, row.WebpayToken.String, nil
}

func (s *PaymentsService) CommitWebpayReturn(ctx context.Context, portalToken, tokenWS string) (*WebpayCommitResult, error) {
	if !s.webpayEnabled() {
		return nil, domain.ErrWebpayNotConfigured
	}
	tokenWS = strings.TrimSpace(tokenWS)
	if tokenWS == "" {
		return &WebpayCommitResult{Status: "failed", Message: "falta token_ws"}, nil
	}

	row, err := s.Repo.GetPaymentTransactionByWebpayToken(ctx, tokenWS)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrWebpayTxNotFound
		}
		return nil, err
	}

	if row.Status == "authorized" {
		return s.commitResultFromRow(row), nil
	}

	commitResp, rawCommit, err := s.Webpay.Transbank.Commit(ctx, tokenWS)
	if err != nil {
		_ = s.Repo.UpdatePaymentTransactionAfterCommit(ctx, row.ID, "failed", &model.CommitUpdate{
			TransbankStatus: "commit_error",
			RawCommit:       rawCommit,
		})
		return &WebpayCommitResult{Status: "failed", Message: "no se pudo confirmar con Transbank"}, nil
	}

	if !s.Webpay.Transbank.Authorized(commitResp) {
		upd := commitUpdateFromResp(commitResp, rawCommit)
		_ = s.Repo.UpdatePaymentTransactionAfterCommit(ctx, row.ID, "failed", upd)
		return &WebpayCommitResult{
			Status:   "failed",
			BuyOrder: commitResp.BuyOrder,
			Message:  "pago no autorizado",
		}, nil
	}

	upd := commitUpdateFromResp(commitResp, rawCommit)
	if err := s.Repo.UpdatePaymentTransactionAfterCommit(ctx, row.ID, "authorized", upd); err != nil {
		return nil, err
	}

	charges, err := s.Repo.ListPaymentTransactionCharges(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	gwRef := commitResp.AuthorizationCode
	for _, link := range charges {
		if err := s.Repo.MarkChargePaidWebpay(ctx, link.ChargeID, link.Amount, row.ID, gwRef); err != nil {
			return nil, err
		}
	}
	_ = s.Repo.MarkPaymentTokenPaid(ctx, portalToken)

	row.Status = "authorized"
	return s.commitResultFromRow(row), nil
}

func (s *PaymentsService) FrontendReturnURL(portalToken string, result *WebpayCommitResult) string {
	base := strings.TrimSuffix(s.Webpay.FrontendBaseURL, "/")
	if base == "" {
		base = "http://localhost:5173"
	}
	q := url.Values{}
	if result != nil {
		q.Set("estado", result.Status)
		if result.BuyOrder != "" {
			q.Set("buy_order", result.BuyOrder)
		}
		if result.AuthorizationCode != "" {
			q.Set("authorization_code", result.AuthorizationCode)
		}
		if result.Amount > 0 {
			q.Set("amount", fmt.Sprintf("%.0f", result.Amount))
		}
		if result.Message != "" {
			q.Set("message", result.Message)
		}
	}
	return base + "/pay/" + url.PathEscape(strings.TrimSpace(portalToken)) + "/return?" + q.Encode()
}

func (s *PaymentsService) resolvePortalCheckout(ctx context.Context, portalToken string, refs []string) (*model.PaymentToken, []model.Charge, int64, error) {
	if len(refs) == 0 {
		return nil, nil, 0, domain.ErrInvalidChargeRefs
	}
	pt, err := s.Repo.GetPaymentTokenByValue(ctx, portalToken)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, 0, domain.ErrPaymentTokenNotFound
		}
		return nil, nil, 0, err
	}
	if pt.Status == model.PaymentTokenStatusRevoked {
		return nil, nil, 0, domain.ErrPaymentTokenNotFound
	}
	all, err := s.Repo.ListChargesByClient(ctx, pt.CompanyID, pt.ClientID)
	if err != nil {
		return nil, nil, 0, err
	}
	want := make(map[string]struct{}, len(refs))
	for _, r := range refs {
		want[strings.TrimSpace(r)] = struct{}{}
	}
	var selected []model.Charge
	var total float64
	for _, ch := range all {
		if ch.PaidAt != nil {
			continue
		}
		ref := encodePortalChargeRef(portalToken, ch.ID)
		if _, ok := want[ref]; ok {
			selected = append(selected, ch)
			total += ch.Amount
			delete(want, ref)
		}
	}
	if len(want) > 0 || len(selected) == 0 {
		return nil, nil, 0, domain.ErrInvalidChargeRefs
	}
	amountCLP := int64(math.Round(total))
	if amountCLP <= 0 {
		return nil, nil, 0, errors.New("monto inválido")
	}
	return pt, selected, amountCLP, nil
}

func newBuyOrder(companyID int64) (string, error) {
	var rb [4]byte
	if _, err := rand.Read(rb[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("FP%06d%010d%s", companyID%1_000_000, time.Now().Unix()%10_000_000_000, hex.EncodeToString(rb[:])), nil
}

func commitUpdateFromResp(resp *transbank.CommitResponse, raw json.RawMessage) *model.CommitUpdate {
	var inst int16
	if resp.InstallmentsNumber > 0 {
		inst = int16(resp.InstallmentsNumber)
	}
	return &model.CommitUpdate{
		AuthorizationCode:  resp.AuthorizationCode,
		PaymentTypeCode:    resp.PaymentTypeCode,
		InstallmentsNumber: inst,
		CardLast4:          transbank.CardLast4(cardNumber(resp)),
		ResponseCode:       int32(resp.ResponseCode),
		TransbankStatus:    resp.Status,
		RawCommit:          raw,
	}
}

func cardNumber(resp *transbank.CommitResponse) string {
	if resp != nil && resp.CardDetail != nil {
		return resp.CardDetail.CardNumber
	}
	return ""
}

func (s *PaymentsService) commitResultFromRow(row *model.PaymentTransaction) *WebpayCommitResult {
	out := &WebpayCommitResult{
		Status:   "failed",
		BuyOrder: row.BuyOrder,
		Amount:   row.Amount,
	}
	if row.Status == "authorized" {
		out.Status = "authorized"
		out.Message = "Pago autorizado"
		if row.AuthorizationCode.Valid {
			out.AuthorizationCode = row.AuthorizationCode.String
		}
	}
	return out
}
