package khipu

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestReconciledRequiresConciliationDate(t *testing.T) {
	if (&Payment{Status: "done"}).Reconciled() {
		t.Fatal("un estado suelto no alcanza")
	}
	if !(&Payment{ConciliationDate: "2024-04-18T13:56:54.859Z"}).Reconciled() {
		t.Fatal("la conciliación es el pago")
	}
	if !(&Payment{Status: "expired", ConciliationDate: "2024-04-18T13:56:54.859Z"}).Reconciled() {
		t.Fatal("conciliado aunque el estado diga otra cosa")
	}
	if (&Payment{Status: "expired"}).Reconciled() {
		t.Fatal("vencido no está conciliado")
	}
	if !(&Payment{Status: "expired"}).Terminal() {
		t.Fatal("vencido es terminal")
	}
	if (&Payment{ConciliationDate: "2024-04-18T13:56:54.859Z", Status: "expired"}).Terminal() {
		t.Fatal("conciliado no es un rechazo")
	}
}

func TestAmountAcceptsNumberOrString(t *testing.T) {
	var number Payment
	if err := json.Unmarshal([]byte(`{"amount":1000}`), &number); err != nil || float64(number.Amount) != 1000 {
		t.Fatalf("número: %+v %v", number, err)
	}
	var text Payment
	if err := json.Unmarshal([]byte(`{"amount":"1000.0000"}`), &text); err != nil || float64(text.Amount) != 1000 {
		t.Fatalf("texto: %+v %v", text, err)
	}
}

func TestSignatureUsesRawBody(t *testing.T) {
	secret := "1a4cbbbeb8bdb7e1d73572b9cc43ce4ce18f79d9"
	body := []byte(`{"payment_id":"zfxnocsow6mz","amount":"1000.0000"}`)
	stamp := "1711965600393"
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(stamp + "."))
	_, _ = mac.Write(body)
	header := "t=" + stamp + ",s=" + base64.StdEncoding.EncodeToString(mac.Sum(nil))

	if !VerifySignature(header, secret, body) {
		t.Fatal("la firma del cuerpo crudo no cerró")
	}
	if VerifySignature(header, "otra-clave", body) {
		t.Fatal("aceptó otro secreto")
	}
	spaced := []byte(`{"payment_id": "zfxnocsow6mz","amount":"1000.0000"}`)
	if VerifySignature(header, secret, spaced) {
		t.Fatal("aceptó un cuerpo distinto")
	}
}
