package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/flowpay/flowpay-payments/internal/model"
)

const paymentTransactionSelect = `
SELECT id, company_id, client_id, payment_token_id,
       gateway, environment, buy_order, session_id, webpay_token,
       amount, currency, status::text,
       authorization_code, payment_type_code, installments_number, card_last4,
       response_code, transbank_status, webpay_redirect_url, return_url,
       raw_create, raw_commit, created_at, updated_at, committed_at
FROM payment_transactions
`

func (db *DB) InsertPaymentTransaction(ctx context.Context, row *model.PaymentTransaction) (int64, error) {
	var id int64
	err := db.sql.QueryRowContext(ctx, `
INSERT INTO payment_transactions (
  company_id, client_id, payment_token_id,
  gateway, environment, buy_order, session_id,
  amount, currency, status, return_url
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::payment_tx_status,$11)
RETURNING id
`, row.CompanyID, row.ClientID, nullInt64(row.PaymentTokenID),
		row.Gateway, row.Environment, row.BuyOrder, row.SessionID,
		row.Amount, row.Currency, row.Status, row.ReturnURL,
	).Scan(&id)
	return id, err
}

func (db *DB) FailPaymentTransaction(ctx context.Context, id int64, note string, raw json.RawMessage) error {
	_, err := db.sql.ExecContext(ctx, `
UPDATE payment_transactions
SET status = 'failed'::payment_tx_status,
    transbank_status = NULLIF($2, ''),
    raw_create = COALESCE($3::jsonb, raw_create),
    updated_at = NOW()
WHERE id = $1
`, id, note, nullableJSON(raw))
	return err
}

func (db *DB) UpdatePaymentTransactionWebpayCreate(ctx context.Context, id int64, webpayToken, webpayURL, status string, rawCreate json.RawMessage) error {
	_, err := db.sql.ExecContext(ctx, `
UPDATE payment_transactions
SET webpay_token = $2,
    status = $3::payment_tx_status,
    raw_create = $4::jsonb,
    webpay_redirect_url = NULLIF($5, ''),
    updated_at = NOW()
WHERE id = $1
`, id, webpayToken, status, nullableJSON(rawCreate), strings.TrimSpace(webpayURL))
	return err
}

func (db *DB) UpdatePaymentTransactionAfterCommit(ctx context.Context, id int64, status string, resp *model.CommitUpdate) error {
	_, err := db.sql.ExecContext(ctx, `
UPDATE payment_transactions
SET status = $2::payment_tx_status,
    authorization_code = $3,
    payment_type_code = $4,
    installments_number = $5,
    card_last4 = $6,
    response_code = $7,
    transbank_status = $8,
    raw_commit = $9::jsonb,
    committed_at = NOW(),
    updated_at = NOW()
WHERE id = $1
`, id, status,
		nullStringVal(resp.AuthorizationCode), nullStringVal(resp.PaymentTypeCode),
		nullInt16Val(resp.InstallmentsNumber), nullStringVal(resp.CardLast4),
		nullInt32Val(resp.ResponseCode), nullStringVal(resp.TransbankStatus),
		nullableJSON(resp.RawCommit))
	return err
}

func (db *DB) GetPaymentTransactionByID(ctx context.Context, id int64) (*model.PaymentTransaction, error) {
	return db.scanPaymentTransaction(db.sql.QueryRowContext(ctx, paymentTransactionSelect+` WHERE id = $1`, id))
}

func (db *DB) GetPaymentTransactionByWebpayToken(ctx context.Context, token string) (*model.PaymentTransaction, error) {
	return db.scanPaymentTransaction(db.sql.QueryRowContext(ctx, paymentTransactionSelect+` WHERE webpay_token = $1`, token))
}

func (db *DB) ListPaymentTransactionCharges(ctx context.Context, transactionID int64) ([]model.PaymentTransactionCharge, error) {
	rows, err := db.sql.QueryContext(ctx, `
SELECT transaction_id, charge_id, amount
FROM payment_transaction_charges
WHERE transaction_id = $1
`, transactionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.PaymentTransactionCharge
	for rows.Next() {
		var row model.PaymentTransactionCharge
		if err := rows.Scan(&row.TransactionID, &row.ChargeID, &row.Amount); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (db *DB) InsertPaymentTransactionCharges(ctx context.Context, transactionID int64, items []model.PaymentTransactionCharge) error {
	for _, it := range items {
		if _, err := db.sql.ExecContext(ctx, `
INSERT INTO payment_transaction_charges (transaction_id, charge_id, amount)
VALUES ($1, $2, $3)
`, transactionID, it.ChargeID, it.Amount); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) scanPaymentTransaction(row *sql.Row) (*model.PaymentTransaction, error) {
	var out model.PaymentTransaction
	var rawCreateNS, rawCommitNS sql.NullString
	err := row.Scan(
		&out.ID, &out.CompanyID, &out.ClientID, &out.PaymentTokenID,
		&out.Gateway, &out.Environment, &out.BuyOrder, &out.SessionID, &out.WebpayToken,
		&out.Amount, &out.Currency, &out.Status,
		&out.AuthorizationCode, &out.PaymentTypeCode, &out.InstallmentsNumber, &out.CardLast4,
		&out.ResponseCode, &out.TransbankStatus, &out.WebpayRedirectURL, &out.ReturnURL,
		&rawCreateNS, &rawCommitNS, &out.CreatedAt, &out.UpdatedAt, &out.CommittedAt,
	)
	if err != nil {
		return nil, err
	}
	if rawCreateNS.Valid {
		out.RawCreate = json.RawMessage(rawCreateNS.String)
	}
	if rawCommitNS.Valid {
		out.RawCommit = json.RawMessage(rawCommitNS.String)
	}
	return &out, nil
}
