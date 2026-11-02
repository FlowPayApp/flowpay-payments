package service

import (
	"context"
	"log"
	"strings"

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

func (s *PaymentsService) noteGateway(ctx context.Context, action, outcome string, companyID, txID int64, role, summary, code, reason string, meta map[string]any) {
	if meta == nil {
		meta = map[string]any{}
	}
	reason = strings.TrimSpace(reason)
	if code != "" {
		meta["code"] = code
	}
	if reason != "" {
		meta["reason"] = reason
		if !strings.Contains(summary, reason) {
			summary = strings.TrimSpace(summary) + ": " + reason
		}
	}
	s.record(ctx, audit.Event{
		Domain:     audit.DomainPayment,
		Action:     action,
		Outcome:    outcome,
		ActorRole:  role,
		CompanyID:  companyID,
		TargetType: audit.TargetPayment,
		TargetID:   audit.ID(txID),
		Summary:    summary,
		Meta:       meta,
	})
}

func gatewayStatusPhrase(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "":
		return "la pasarela no informó el estado"
	case "failed":
		return "el pago falló"
	case "rejected":
		return "el banco rechazó el pago"
	case "expired":
		return "el intento venció"
	case "canceled", "cancelled":
		return "el intento se canceló"
	default:
		return "estado " + clipStatus(status)
	}
}

func chargeIDs(charges []model.Charge) []int64 {
	ids := make([]int64, 0, len(charges))
	for _, ch := range charges {
		ids = append(ids, ch.ID)
	}
	return ids
}
