package model

import "time"

const (
	PaymentTokenStatusIssued  = "issued"
	PaymentTokenStatusViewed  = "viewed"
	PaymentTokenStatusPaid    = "paid"
	PaymentTokenStatusRevoked = "revoked"
)

type PaymentToken struct {
	ID        int64     `json:"id"`
	CompanyID int64     `json:"company_id"`
	ClientID  int64     `json:"client_id"`
	Token     string    `json:"token"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}
