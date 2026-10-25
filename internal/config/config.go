package config

import (
	"log"
	"os"
	"strings"

	"github.com/flowpay/flowpay-payments/internal/secretbox"
)

type Config struct {
	DSN                   string
	Addr                  string
	DefaultCompanyID      int64
	JWTSecret             string
	PublicBaseURL         string
	FrontendBaseURL       string
	TransbankEnvironment  string
	PaymentCredentialsKey []byte
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
	addr := listenAddr("FLOWPAY_PAYMENTS_ADDR", ":8081")
	tbkEnv := envFirst("FLOWPAY_PAYMENTS_TRANSBANK_ENV", "FLOWPAY_TRANSBANK_ENV")
	if tbkEnv == "" {
		tbkEnv = "integration"
	}
	frontendBase := envTrimSuffix("FLOWPAY_PAYMENTS_FRONTEND_BASE_URL", "FLOWPAY_FRONTEND_BASE_URL")
	if frontendBase == "" {
		if onPlatform() {
			frontendBase = productionOrigin
		} else {
			frontendBase = "http://localhost:5173"
		}
	}
	return Config{
		DSN:                   dsn,
		Addr:                  addr,
		DefaultCompanyID:      1,
		JWTSecret:             envFirst("FLOWPAY_PAYMENTS_JWT_SECRET", "FLOWPAY_JWT_SECRET"),
		PublicBaseURL:         publicURL("FLOWPAY_PAYMENTS_PUBLIC_BASE_URL", "FLOWPAY_PUBLIC_BASE_URL"),
		FrontendBaseURL:       frontendBase,
		TransbankEnvironment:  tbkEnv,
		PaymentCredentialsKey: paymentCredentialsKey(),
	}
}

func paymentCredentialsKey() []byte {
	key, err := secretbox.KeyFromMaterial(os.Getenv("FLOWPAY_PAYMENT_CREDENTIALS_KEY"))
	if err != nil {
		log.Println("warn: FLOWPAY_PAYMENT_CREDENTIALS_KEY no está configurada; el enlace no puede cobrar con tarjeta")
		return nil
	}
	return key
}

const productionOrigin = "https://geldflus.com"

func onPlatform() bool {
	return strings.TrimSpace(os.Getenv("PORT")) != ""
}

func listenAddr(primaryKey, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(primaryKey)); v != "" {
		return v
	}
	if p := strings.TrimSpace(os.Getenv("PORT")); p != "" {
		if strings.HasPrefix(p, ":") {
			return p
		}
		return ":" + p
	}
	return fallback
}

func publicURL(keys ...string) string {
	v := envTrimSuffix(keys...)
	if v != "" {
		return v
	}
	if onPlatform() {
		return productionOrigin
	}
	return ""
}
