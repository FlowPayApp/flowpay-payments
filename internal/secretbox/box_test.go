package secretbox

import (
	"bytes"
	"strings"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	key, err := KeyFromMaterial("esta-es-una-clave-de-prueba-de-32+")
	if err != nil {
		t.Fatal(err)
	}
	aad := []byte("7|webpay_api_key")
	plain := []byte("llave-de-la-empresa")
	sealed, err := Seal(key, plain, aad)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed, "v1.") {
		t.Fatalf("formato: %s", sealed)
	}
	if bytes.Contains(plain, []byte("llave-de-la")) {
		t.Fatal("el texto en claro quedó en el buffer")
	}
	got, err := Open(key, sealed, aad)
	if err != nil {
		t.Fatal(err)
	}
	if got != "llave-de-la-empresa" {
		t.Fatalf("got %q", got)
	}
}

func TestOpenRejectsSwappedContext(t *testing.T) {
	key, err := KeyFromMaterial("esta-es-una-clave-de-prueba-de-32+")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := Seal(key, []byte("llave-de-la-empresa"), []byte("7|webpay_api_key"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(key, sealed, []byte("8|webpay_api_key")); err == nil {
		t.Fatal("aceptó el secreto de otra empresa")
	}
	if _, err := Open(key, sealed, []byte("7|webpay_commerce_code")); err == nil {
		t.Fatal("aceptó el secreto en otra columna")
	}
}
