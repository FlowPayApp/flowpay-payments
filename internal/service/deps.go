package service

import (
	"github.com/flowpay/flowpay-payments/internal/repository"
)

type WebpayConfig struct {
	PublicBaseURL   string
	FrontendBaseURL string
	Environment     string
}

type PaymentsService struct {
	Repo       *repository.DB
	Webpay     *WebpayConfig
	PaymentKey []byte
}
