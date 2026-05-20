package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/flowpay/flowpay-payments/internal/domain"
	"github.com/flowpay/flowpay-payments/internal/model"
)

type PaymentPortalCompany struct {
	Name                 string `json:"name"`
	TransferInstructions string `json:"transfer_instructions,omitempty"`
}

type PaymentPortalClient struct {
	Label string `json:"label"`
}

type PaymentPortalResponse struct {
	TokenStatus string               `json:"token_status"`
	IssuedAt    time.Time            `json:"issued_at"`
	Company     PaymentPortalCompany `json:"company"`
	Client      PaymentPortalClient  `json:"client"`
	Charges     []PortalCharge       `json:"charges"`
	Totals      PaymentPortalTotals  `json:"totals"`
}

type PaymentPortalTotals struct {
	Pending float64 `json:"pending"`
	Overdue float64 `json:"overdue"`
	Paid    float64 `json:"paid"`
}

type PortalCharge struct {
	Ref             string    `json:"ref"`
	Amount          float64   `json:"amount"`
	DueDate         time.Time `json:"due_date"`
	Status          string    `json:"status"`
	AttachmentToken *string   `json:"attachment_token,omitempty"`
}

func (s *PaymentsService) ResolvePaymentPortal(ctx context.Context, tokenValue string) (*PaymentPortalResponse, error) {
	tokenValue = strings.TrimSpace(tokenValue)
	if tokenValue == "" {
		return nil, domain.ErrPaymentTokenNotFound
	}
	row, err := s.Repo.GetPaymentTokenByValue(ctx, tokenValue)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrPaymentTokenNotFound
		}
		return nil, err
	}
	if row.Status == model.PaymentTokenStatusIssued {
		if changed, mErr := s.Repo.MarkPaymentTokenViewed(ctx, row.Token); mErr == nil && changed {
			row.Status = model.PaymentTokenStatusViewed
		}
	}
	cm, err := s.Repo.GetCompanyMessaging(ctx, row.CompanyID)
	if err != nil {
		return nil, err
	}
	label, err := s.Repo.GetClientLabel(ctx, row.CompanyID, row.ClientID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			label = ""
		} else {
			return nil, err
		}
	}
	charges, err := s.Repo.ListChargesByClient(ctx, row.CompanyID, row.ClientID)
	if err != nil {
		return nil, err
	}
	out := &PaymentPortalResponse{
		TokenStatus: row.Status,
		IssuedAt:    row.CreatedAt,
		Company: PaymentPortalCompany{
			Name:                 cm.Name,
			TransferInstructions: cm.TransferInstructions,
		},
		Client:  PaymentPortalClient{Label: label},
		Charges: make([]PortalCharge, 0, len(charges)),
	}
	now := time.Now()
	for _, ch := range charges {
		st := domain.ChargeStatus(ch.PaidAt, ch.DueDate, now)
		pc := PortalCharge{
			Ref:             encodePortalChargeRef(row.Token, ch.ID),
			Amount:          ch.Amount,
			DueDate:         ch.DueDate,
			Status:          st,
			AttachmentToken: ch.AttachmentToken,
		}
		out.Charges = append(out.Charges, pc)
		switch st {
		case "paid":
			out.Totals.Paid += ch.Amount
		case "overdue":
			out.Totals.Overdue += ch.Amount
		default:
			out.Totals.Pending += ch.Amount
		}
	}
	return out, nil
}

func encodePortalChargeRef(tokenValue string, chargeID int64) string {
	mac := hmac.New(sha256.New, []byte(tokenValue))
	mac.Write([]byte(strconv.FormatInt(chargeID, 10)))
	sum := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(sum[:12])
}
