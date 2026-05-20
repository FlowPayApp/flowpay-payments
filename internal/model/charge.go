package model

import "time"

type Charge struct {
	ID              int64
	CompanyID       int64
	ClientID        int64
	Amount          float64
	DueDate         time.Time
	PaidAt          *time.Time
	CreatedAt       time.Time
	AttachmentToken *string
}
