package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/flowpay/flowpay-payments/internal/model"
	"github.com/jackc/pgx/v5/pgconn"
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

// CompanyPaymentMethods dice qué medios en línea tiene cargados la empresa.
// No lee los secretos. Si la tabla todavía no existe, no hay medios.
func (db *DB) CompanyPaymentMethods(ctx context.Context, companyID int64) (webpay, khipu, fintoc bool, err error) {
	err = db.sql.QueryRowContext(ctx, `
SELECT
	webpay_commerce_code IS NOT NULL AND webpay_api_key IS NOT NULL,
	khipu_api_key IS NOT NULL,
	fintoc_secret_key IS NOT NULL
FROM company_payment_credentials
WHERE company_id = $1
`, companyID).Scan(&webpay, &khipu, &fintoc)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, false, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
		return false, false, false, nil
	}
	return webpay, khipu, fintoc, err
}

// CompanyWebpaySealed devuelve el código y la llave cifrados. ok es falso si faltan.
func (db *DB) CompanyWebpaySealed(ctx context.Context, companyID int64) (commerce, apiKey string, ok bool, err error) {
	var code, key sql.NullString
	err = db.sql.QueryRowContext(ctx, `
SELECT webpay_commerce_code, webpay_api_key
FROM company_payment_credentials
WHERE company_id = $1
`, companyID).Scan(&code, &key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	if !code.Valid || !key.Valid || strings.TrimSpace(code.String) == "" || strings.TrimSpace(key.String) == "" {
		return "", "", false, nil
	}
	return code.String, key.String, true, nil
}

// CompanyKhipuSealed devuelve la API key cifrada. ok es falso si falta.
func (db *DB) CompanyKhipuSealed(ctx context.Context, companyID int64) (apiKey string, ok bool, err error) {
	var key sql.NullString
	err = db.sql.QueryRowContext(ctx, `
SELECT khipu_api_key
FROM company_payment_credentials
WHERE company_id = $1
`, companyID).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !key.Valid || strings.TrimSpace(key.String) == "" {
		return "", false, nil
	}
	return key.String, true, nil
}
