package service

import (
	"context"
	"log"

	"github.com/flowpay/flowpay-payments/internal/audit"
	"github.com/flowpay/flowpay-payments/internal/model"
)

func (s *PaymentsService) record(ctx context.Context, e audit.Event) {
	if s == nil || s.Repo == nil {
		return
	}
	if err := s.Repo.RecordAudit(ctx, e); err != nil {
		log.Printf("[FlowPay audit] %s.%s: %v", e.Domain, e.Action, err)
	}
}

func chargeIDs(charges []model.Charge) []int64 {
	ids := make([]int64, 0, len(charges))
	for _, ch := range charges {
		ids = append(ids, ch.ID)
	}
	return ids
}
