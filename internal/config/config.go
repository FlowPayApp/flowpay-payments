package config

import (
	"os"
	"strings"
)

type Config struct {
	DSN                   string
	Addr                  string
	DefaultCompanyID      int64
	JWTSecret             string
	PublicBaseURL         string
	FrontendBaseURL       string
	TransbankCommerceCode string
	TransbankAPIKey       string
	TransbankEnvironment  string
}

func envFirst(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func envTrimSuffix(keys ...string) string {
	v := envFirst(keys...)
	return strings.TrimSuffix(v, "/")
}

func Load() Config {
	dsn := envFirst("FLOWPAY_PAYMENTS_DSN", "FLOWPAY_DSN")
	if dsn == "" {
		dsn = "postgres://flowpay:flowpay@127.0.0.1:5432/flowpay?sslmode=disable"
	}
	addr := envFirst("FLOWPAY_PAYMENTS_ADDR")
	if addr == "" {
		addr = ":8081"
	}
	tbkEnv := envFirst("FLOWPAY_PAYMENTS_TRANSBANK_ENV", "FLOWPAY_TRANSBANK_ENV")
	if tbkEnv == "" {
		tbkEnv = "integration"
	}
	frontendBase := envTrimSuffix("FLOWPAY_PAYMENTS_FRONTEND_BASE_URL", "FLOWPAY_FRONTEND_BASE_URL")
	if frontendBase == "" {
		frontendBase = "http://localhost:5173"
	}
	return Config{
		DSN:                   dsn,
		Addr:                  addr,
		DefaultCompanyID:      1,
		JWTSecret:             envFirst("FLOWPAY_PAYMENTS_JWT_SECRET", "FLOWPAY_JWT_SECRET"),
		PublicBaseURL:         envTrimSuffix("FLOWPAY_PAYMENTS_PUBLIC_BASE_URL", "FLOWPAY_PUBLIC_BASE_URL"),
		FrontendBaseURL:       frontendBase,
		TransbankCommerceCode: envFirst("FLOWPAY_PAYMENTS_TRANSBANK_COMMERCE_CODE", "FLOWPAY_TRANSBANK_COMMERCE_CODE"),
		TransbankAPIKey:       envFirst("FLOWPAY_PAYMENTS_TRANSBANK_API_KEY", "FLOWPAY_TRANSBANK_API_KEY"),
		TransbankEnvironment:  tbkEnv,
	}
}
