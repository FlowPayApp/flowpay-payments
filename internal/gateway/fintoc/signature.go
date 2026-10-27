package fintoc

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

const signatureTolerance = 5 * time.Minute

// VerifySignature comprueba Fintoc-Signature sobre el cuerpo crudo.
// El mensaje firmado es timestamp + "." + body. La firma v1 es HMAC-SHA256 en hex.
func VerifySignature(header, secret string, rawBody []byte, now time.Time) bool {
	secret = strings.TrimSpace(secret)
	if secret == "" || header == "" || len(rawBody) == 0 {
		return false
	}
	stamp, sig := splitSignature(header)
	if stamp == "" || sig == "" {
		return false
	}
	unix, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return false
	}
	age := now.Sub(time.Unix(unix, 0))
	if age < 0 {
		age = -age
	}
	if age > signatureTolerance {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(stamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(rawBody)
	got, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
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
		case "v1":
			sig = strings.TrimSpace(value)
		}
	}
	return stamp, sig
}
