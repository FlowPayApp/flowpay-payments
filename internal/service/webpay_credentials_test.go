package service

import (
	"testing"

	"github.com/flowpay/flowpay-payments/internal/domain"
	"github.com/flowpay/flowpay-payments/internal/secretbox"
)

func TestWebpayClientUsesCompanyCredential(t *testing.T) {
	key, err := secretbox.KeyFromMaterial("esta-es-una-clave-de-prueba-de-32+")
	if err != nil {
		t.Fatal(err)
	}
	const companyID int64 = 7
	commerce, err := secretbox.Seal(key, []byte("597012345678"), webpayAAD(companyID, "webpay_commerce_code"))
	if err != nil {
		t.Fatal(err)
	}
	apiKey, err := secretbox.Seal(key, []byte("llave-propia-de-la-empresa"), webpayAAD(companyID, "webpay_api_key"))
	if err != nil {
		t.Fatal(err)
	}

	client, err := webpayClientFromSecrets(key, companyID, commerce, apiKey, "integration")
	if err != nil {
		t.Fatal(err)
	}
	if client.CommerceCode != "597012345678" || client.APIKey != "llave-propia-de-la-empresa" {
		t.Fatal("el cliente no quedó con la credencial de la empresa")
	}
	if client.CommerceCode == "597055555532" {
		t.Fatal("usó el código de comercio del servidor")
	}

	if _, err := webpayClientFromSecrets(key, companyID+1, commerce, apiKey, "integration"); err != domain.ErrWebpayCredential {
		t.Fatalf("otra empresa: %v", err)
	}
	if _, err := webpayClientFromSecrets(nil, companyID, commerce, apiKey, "integration"); err != domain.ErrWebpayCredential {
		t.Fatalf("sin clave de cifrado: %v", err)
	}
}
