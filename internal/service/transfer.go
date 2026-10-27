package service

import (
	"context"

	"github.com/flowpay/flowpay-payments/internal/domain"
)

const (
	khipuGateway  = "khipu"
	fintocGateway = "fintoc"
)

// transferOffered es verdadero cuando esa credencial está cargada. Khipu y Fintoc se ofrecen juntas.
func transferOffered(khipuOn, fintocOn bool, want string) bool {
	switch want {
	case khipuGateway:
		return khipuOn
	case fintocGateway:
		return fintocOn
	default:
		return false
	}
}

func (s *PaymentsService) requireTransfer(ctx context.Context, companyID int64, want string) error {
	_, khipuOn, fintocOn, _, err := s.Repo.CompanyPaymentMethods(ctx, companyID)
	if err != nil {
		return err
	}
	if transferOffered(khipuOn, fintocOn, want) {
		return nil
	}
	if want == fintocGateway {
		return domain.ErrFintocNotOffered
	}
	return domain.ErrKhipuNotOffered
}
