package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPublicPaymentLimitAllowsAPaymentAndBlocksABurst(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ok := func(c *gin.Context) { c.Status(http.StatusNoContent) }

	normal := gin.New()
	limit := PublicPaymentLimit(nil)
	normal.GET("/api/public/pay/:token", limit, ok)
	normal.GET("/api/public/webpay/return/:token", limit, ok)
	normal.POST("/api/public/webpay/return/:token", limit, ok)
	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/api/public/pay/tok"},
		{http.MethodPost, "/api/public/webpay/return/tok"},
		{http.MethodGet, "/api/public/pay/tok"},
	} {
		if rec := hitPublic(normal, call.method, call.path, "203.0.113.10"); rec.Code != http.StatusNoContent {
			t.Fatalf("pago normal %s %s: %d", call.method, call.path, rec.Code)
		}
	}

	burst := gin.New()
	var noted int
	shared := PublicPaymentLimit(func(*gin.Context) { noted++ })
	burst.GET("/api/public/pay/:token", shared, ok)
	burst.POST("/api/public/webpay/return/:token", shared, ok)
	for i := 0; i < publicPayMax; i++ {
		method, path := http.MethodGet, "/api/public/pay/tok"
		if i%2 == 1 {
			method, path = http.MethodPost, "/api/public/webpay/return/tok"
		}
		if rec := hitPublic(burst, method, path, "203.0.113.20"); rec.Code != http.StatusNoContent {
			t.Fatalf("dentro del tope %d: %d", i, rec.Code)
		}
	}
	if rec := hitPublic(burst, http.MethodGet, "/api/public/pay/tok", "203.0.113.20"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("ráfaga del portal: %d", rec.Code)
	}
	if rec := hitPublic(burst, http.MethodPost, "/api/public/webpay/return/tok", "203.0.113.20"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("ráfaga del retorno: %d", rec.Code)
	}
	if noted != 1 {
		t.Fatalf("la auditoría del tope debe anotarse una vez, hubo %d", noted)
	}
	if rec := hitPublic(burst, http.MethodGet, "/api/public/pay/tok", "203.0.113.30"); rec.Code != http.StatusNoContent {
		t.Fatalf("otra IP: %d", rec.Code)
	}
}

func hitPublic(r *gin.Engine, method, path, ip string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Flowpay-Client-Ip", ip)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}
