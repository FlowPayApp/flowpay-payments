package khipu

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// VerifySignature comprueba x-khipu-signature sobre el cuerpo crudo.
// El secreto es el de la cuenta que creó el cobro. No altera el JSON.
func VerifySignature(header, secret string, rawBody []byte) bool {
	secret = strings.TrimSpace(secret)
	if secret == "" || header == "" || len(rawBody) == 0 {
		return false
	}
	stamp, sig := splitSignature(header)
	if stamp == "" || sig == "" {
		return false
	}
	got, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(stamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(rawBody)
	return hmac.Equal(mac.Sum(nil), got)
}

func splitSignature(header string) (stamp, sig string) {
	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "t":
			stamp = strings.TrimSpace(value)
		case "s":
			sig = strings.TrimSpace(value)
		}
	}
	return stamp, sig
}
