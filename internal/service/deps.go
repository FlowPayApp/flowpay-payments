package service

import (
	"github.com/flowpay/flowpay-payments/internal/gateway/transbank"
	"github.com/flowpay/flowpay-payments/internal/repository"
)

type WebpayConfig struct {
	PublicBaseURL   string
	FrontendBaseURL string
	Environment     string
	Transbank       *transbank.Client
}

type PaymentsService struct {
	Repo   *repository.DB
	Webpay *WebpayConfig
}

func (s *PaymentsService) webpayEnabled() bool {
	return s.Webpay != nil && s.Webpay.Transbank != nil && s.Webpay.Transbank.Enabled()
}
