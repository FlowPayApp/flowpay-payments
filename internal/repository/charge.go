package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/flowpay/flowpay-payments/internal/model"
)

const chargeClientLabelExpr = `COALESCE(NULLIF(TRIM(COALESCE(c.branch_name, '')), ''), 'Sin sucursal')`

func (db *DB) ListChargesByClient(ctx context.Context, companyID, clientID int64) ([]model.Charge, error) {
	q := `
SELECT i.id, i.company_id, i.client_id, i.amount, i.due_date, i.paid_at,
       i.attachment_token, i.created_at
FROM charges i
JOIN clients c ON c.id = i.client_id
WHERE i.company_id = $1 AND i.client_id = $2
ORDER BY i.due_date ASC, i.id ASC
`
	rows, err := db.sql.QueryContext(ctx, q, companyID, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Charge
	for rows.Next() {
		ch, err := scanCharge(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

func (db *DB) GetCharge(ctx context.Context, companyID, chargeID int64) (*model.Charge, error) {
	q := `
SELECT i.id, i.company_id, i.client_id, i.amount, i.due_date, i.paid_at,
       i.attachment_token, i.created_at
FROM charges i
WHERE i.company_id = $1 AND i.id = $2
`
	row := db.sql.QueryRowContext(ctx, q, companyID, chargeID)
	ch, err := scanChargeRow(row)
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

func (db *DB) MarkChargePaidManual(ctx context.Context, chargeID int64, amount float64) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO payments (charge_id, amount, method) VALUES ($1, $2, 'manual')`, chargeID, amount); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE charges SET paid_at = CURRENT_TIMESTAMP WHERE id = $1`, chargeID); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) MarkChargePaidWebpay(ctx context.Context, chargeID int64, amount float64, transactionID int64, gatewayRef string) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO payments (charge_id, amount, transaction_id, method, gateway_reference)
VALUES ($1, $2, $3, 'webpay', NULLIF($4, ''))
`, chargeID, amount, transactionID, gatewayRef); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE charges SET paid_at = CURRENT_TIMESTAMP WHERE id = $1 AND paid_at IS NULL`, chargeID); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) ClientBelongsToCompany(ctx context.Context, companyID, clientID int64) (bool, error) {
	var one int
	err := db.sql.QueryRowContext(ctx,
		`SELECT 1 FROM clients WHERE id = $1 AND company_id = $2 LIMIT 1`,
		clientID, companyID,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (db *DB) GetClientLabel(ctx context.Context, companyID, clientID int64) (string, error) {
	var label string
	err := db.sql.QueryRowContext(ctx,
		`SELECT `+chargeClientLabelExpr+` FROM clients c WHERE c.id = $1 AND c.company_id = $2`,
		clientID, companyID,
	).Scan(&label)
	return label, err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanCharge(rows *sql.Rows) (model.Charge, error) {
	var ch model.Charge
	var paidAt sql.NullTime
	var atok sql.NullString
	if err := rows.Scan(&ch.ID, &ch.CompanyID, &ch.ClientID, &ch.Amount, &ch.DueDate, &paidAt, &atok, &ch.CreatedAt); err != nil {
		return ch, err
	}
	if paidAt.Valid {
		t := paidAt.Time
		ch.PaidAt = &t
	}
	if atok.Valid {
		s := atok.String
		ch.AttachmentToken = &s
	}
	return ch, nil
}

func scanChargeRow(row *sql.Row) (model.Charge, error) {
	var ch model.Charge
	var paidAt sql.NullTime
	var atok sql.NullString
	if err := row.Scan(&ch.ID, &ch.CompanyID, &ch.ClientID, &ch.Amount, &ch.DueDate, &paidAt, &atok, &ch.CreatedAt); err != nil {
		return ch, err
	}
	if paidAt.Valid {
		t := paidAt.Time
		ch.PaidAt = &t
	}
	if atok.Valid {
		s := atok.String
		ch.AttachmentToken = &s
	}
	return ch, nil
}
