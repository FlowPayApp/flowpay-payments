package revoked

import (
	"testing"
	"time"
)

func TestHasDropsExpired(t *testing.T) {
	list := New()
	list.Replace(map[string]time.Time{
		"vivo":    time.Now().Add(time.Minute),
		"vencido": time.Now().Add(-time.Second),
	})
	if !list.Has("vivo") {
		t.Fatal("el jti vigente tiene que estar revocado")
	}
	if list.Has("vencido") || list.Has("") || list.Has("otro") {
		t.Fatal("solo el jti vigente y no vencido cuenta")
	}
}
