package repository

import (
	"context"
	"database/sql"

	"github.com/flowpay/flowpay-payments/internal/model"
)

func (db *DB) GetCompanyMessaging(ctx context.Context, companyID int64) (*model.CompanyMessaging, error) {
	var m model.CompanyMessaging
	var ti sql.NullString
	err := db.sql.QueryRowContext(ctx, `
SELECT name, transfer_instructions
FROM companies WHERE id = $1
`, companyID).Scan(&m.Name, &ti)
	if err != nil {
		return nil, err
	}
	if ti.Valid {
		m.TransferInstructions = ti.String
	}
	return &m, nil
}
