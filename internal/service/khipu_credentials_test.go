package service

import (
	"testing"

	"github.com/flowpay/flowpay-payments/internal/domain"
	"github.com/flowpay/flowpay-payments/internal/secretbox"
)

func TestKhipuNotifyURLSkipsLocalhost(t *testing.T) {
	if got := khipuNotifyURL("http://localhost:8081"); got != "" {
		t.Fatalf("localhost: %q", got)
	}
	if got := khipuNotifyURL("https://localhost:8081"); got != "" {
		t.Fatalf("https localhost: %q", got)
	}
	const want = "https://pay.geldflus.com/api/public/khipu/notify"
	if got := khipuNotifyURL("https://pay.geldflus.com"); got != want {
		t.Fatalf("pública: %q", got)
	}
}

func TestKhipuClientUsesCompanyCredential(t *testing.T) {
	key, err := secretbox.KeyFromMaterial("esta-es-una-clave-de-prueba-de-32+")
	if err != nil {
		t.Fatal(err)
	}
	const companyID int64 = 7
	sealed, err := secretbox.Seal(key, []byte("khipu-api-key-de-la-empresa"), webpayAAD(companyID, "khipu_api_key"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := khipuClientFromSecret(key, companyID, sealed)
	if err != nil {
		t.Fatal(err)
	}
	if client.APIKey != "khipu-api-key-de-la-empresa" {
		t.Fatal("el cliente no quedó con la API key de la empresa")
	}
	if _, err := khipuClientFromSecret(key, companyID+1, sealed); err != domain.ErrKhipuCredential {
		t.Fatalf("otra empresa: %v", err)
	}
	if _, err := khipuClientFromSecret(nil, companyID, sealed); err != domain.ErrKhipuCredential {
		t.Fatalf("sin clave de cifrado: %v", err)
	}
}
