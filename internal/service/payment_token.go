package service

import (
	"context"
	"errors"

	"github.com/flowpay/flowpay-payments/internal/model"
	"github.com/flowpay/flowpay-payments/internal/pkg/paymenttoken"
)

func (s *PaymentsService) IssuePaymentToken(ctx context.Context, companyID, clientID int64) (*model.PaymentToken, error) {
	if companyID <= 0 || clientID <= 0 {
		return nil, errors.New("company_id y client_id deben ser positivos")
	}
	ok, err := s.Repo.ClientBelongsToCompany(ctx, companyID, clientID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("el cliente no pertenece a la empresa")
	}
	tok, err := paymenttoken.NewOpaque()
	if err != nil {
		return nil, err
	}
	return s.Repo.InsertPaymentToken(ctx, companyID, clientID, tok, model.PaymentTokenStatusIssued)
}
