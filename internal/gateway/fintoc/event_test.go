package fintoc

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"
)

func TestVerifySignature(t *testing.T) {
	secret := "whsec_test_fintoc"
	body := []byte(`{"id":"evt_1","type":"checkout_session.finished"}`)
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(stamp + "."))
	_, _ = mac.Write(body)
	header := "t=" + stamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))
	if !VerifySignature(header, secret, body, time.Now()) {
		t.Fatal("la firma válida no pasó")
	}
	if VerifySignature(header, "otro-secreto", body, time.Now()) {
		t.Fatal("aceptó otro secreto")
	}
	old := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	if VerifySignature("t="+old+",v1="+hex.EncodeToString(mac.Sum(nil)), secret, body, time.Now()) {
		t.Fatal("aceptó un aviso viejo")
	}
}

func TestInterpretPaidTransfer(t *testing.T) {
	raw := []byte(`{
		"type": "checkout_session.finished",
		"data": {
			"id": "cs_test",
			"object": "checkout_session",
			"status": "finished",
			"amount": 89000,
			"currency": "CLP",
			"metadata": {"buy_order": "FP000001"},
			"payment_resource": {
				"payment_intent": {
					"id": "pi_test",
					"status": "succeeded",
					"amount": 89000,
					"currency": "CLP",
					"payment_type": "bank_transfer"
				}
			}
		}
	}`)
	got, err := Interpret(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Action != "pay" || got.SessionID != "cs_test" || got.BuyOrder != "FP000001" || got.Amount != 89000 || got.PaymentID != "pi_test" {
		t.Fatalf("%+v", got)
	}
}

func TestInterpretRejectsCardAndRepeatAmount(t *testing.T) {
	raw := []byte(`{
		"type": "checkout_session.finished",
		"data": {
			"id": "cs_card",
			"object": "checkout_session",
			"amount": 1000,
			"currency": "CLP",
			"payment_resource": {"payment_intent": {"id": "pi_card", "status": "succeeded", "amount": 1000, "payment_type": "card"}}
		}
	}`)
	got, err := Interpret(raw)
	if err != nil || got.Action != "mismatch" {
		t.Fatalf("%+v %v", got, err)
	}

	drift := []byte(`{
		"type": "checkout_session.finished",
		"data": {
			"id": "cs_drift",
			"object": "checkout_session",
			"amount": 1000,
			"payment_resource": {"payment_intent": {"status": "succeeded", "amount": 999, "payment_type": "bank_transfer"}}
		}
	}`)
	got, err = Interpret(drift)
	if err != nil || got.Action != "mismatch" {
		t.Fatalf("montos distintos: %+v %v", got, err)
	}
}

func TestInterpretDoesNotPayPending(t *testing.T) {
	raw := []byte(`{"type":"payment_intent.requires_action","data":{"id":"pi_wait","object":"payment_intent","status":"requires_action","amount":1000,"currency":"CLP","payment_type":"bank_transfer","checkout_session_id":"cs_wait","metadata":{"buy_order":"ORD1"}}}`)
	got, err := Interpret(raw)
	if err != nil || got.Action != "ignore" || got.SessionID != "cs_wait" {
		t.Fatalf("%+v %v", got, err)
	}
}
