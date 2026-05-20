package model

import (
	"database/sql"
	"encoding/json"
	"time"
)

type PaymentTransaction struct {
	ID                 int64
	CompanyID          int64
	ClientID           int64
	PaymentTokenID     sql.NullInt64
	Gateway            string
	Environment        string
	BuyOrder           string
	SessionID          string
	WebpayToken        sql.NullString
	Amount             float64
	Currency           string
	Status             string
	AuthorizationCode  sql.NullString
	PaymentTypeCode    sql.NullString
	InstallmentsNumber sql.NullInt16
	CardLast4          sql.NullString
	ResponseCode       sql.NullInt32
	TransbankStatus    sql.NullString
	WebpayRedirectURL  sql.NullString
	ReturnURL          string
	RawCreate          json.RawMessage
	RawCommit          json.RawMessage
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CommittedAt        sql.NullTime
}

type PaymentTransactionCharge struct {
	TransactionID int64
	ChargeID      int64
	Amount        float64
}

type CommitUpdate struct {
	AuthorizationCode  string
	PaymentTypeCode    string
	InstallmentsNumber int16
	CardLast4          string
	ResponseCode       int32
	TransbankStatus    string
	RawCommit          json.RawMessage
}
