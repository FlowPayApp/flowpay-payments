package service

import (
	"context"
	"database/sql"
	"errors"
)

func (s *PaymentsService) RecordManualPayment(ctx context.Context, companyID, chargeID int64, amount float64) error {
	ch, err := s.Repo.GetCharge(ctx, companyID, chargeID)
	if err != nil {
		return err
	}
	if ch.PaidAt != nil {
		return errors.New("already paid")
	}
	if amount < ch.Amount-0.01 {
		return errors.New("amount must cover charge total for MVP")
	}
	return s.Repo.MarkChargePaidManual(ctx, chargeID, amount)
}

func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
