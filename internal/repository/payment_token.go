package repository

import (
	"context"

	"github.com/flowpay/flowpay-payments/internal/model"
)

func (db *DB) InsertPaymentToken(ctx context.Context, companyID, clientID int64, token, status string) (*model.PaymentToken, error) {
	var row model.PaymentToken
	err := db.sql.QueryRowContext(ctx, `
INSERT INTO payment_tokens (company_id, client_id, token, status)
VALUES ($1, $2, $3, $4)
RETURNING id, company_id, client_id, token, status, created_at
`, companyID, clientID, token, status).Scan(
		&row.ID, &row.CompanyID, &row.ClientID, &row.Token, &row.Status, &row.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (db *DB) GetPaymentTokenByValue(ctx context.Context, token string) (*model.PaymentToken, error) {
	var row model.PaymentToken
	err := db.sql.QueryRowContext(ctx, `
SELECT id, company_id, client_id, token, status, created_at
FROM payment_tokens
WHERE trim(both from token) = trim(both from $1::text)
LIMIT 1
`, token).Scan(&row.ID, &row.CompanyID, &row.ClientID, &row.Token, &row.Status, &row.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (db *DB) MarkPaymentTokenViewed(ctx context.Context, token string) (bool, error) {
	res, err := db.sql.ExecContext(ctx, `
UPDATE payment_tokens
SET status = 'viewed'::payment_token_status
WHERE trim(both from token) = trim(both from $1::text)
  AND status = 'issued'::payment_token_status
`, token)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (db *DB) MarkPaymentTokenPaid(ctx context.Context, token string) error {
	_, err := db.sql.ExecContext(ctx, `
UPDATE payment_tokens
SET status = 'paid'::payment_token_status
WHERE trim(both from token) = trim(both from $1::text)
  AND status IN ('issued'::payment_token_status, 'viewed'::payment_token_status)
`, token)
	return err
}
