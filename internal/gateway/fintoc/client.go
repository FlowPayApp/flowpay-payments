// Package fintoc habla con la Checkout Session de Fintoc para una transferencia.
package fintoc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const baseURL = "https://api.fintoc.com"

type Client struct {
	SecretKey string
	HTTP      *http.Client
}

func NewClient(secretKey string) *Client {
	return &Client{
		SecretKey: strings.TrimSpace(secretKey),
		HTTP:      &http.Client{Timeout: 25 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.SecretKey != ""
}

type CreateSessionRequest struct {
	Amount             int64             `json:"amount"`
	Currency           string            `json:"currency"`
	SuccessURL         string            `json:"success_url"`
	CancelURL          string            `json:"cancel_url"`
	PaymentMethodTypes []string          `json:"payment_method_types"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

type Session struct {
	ID          string `json:"id"`
	RedirectURL string `json:"redirect_url"`
	Status      string `json:"status"`
}

func (c *Client) CreateSession(ctx context.Context, req CreateSessionRequest) (*Session, []byte, error) {
	var out Session
	raw, err := c.do(ctx, http.MethodPost, "/v2/checkout_sessions", req, &out)
	if err != nil {
		return nil, raw, err
	}
	if strings.TrimSpace(out.ID) == "" || strings.TrimSpace(out.RedirectURL) == "" {
		return nil, raw, fmt.Errorf("fintoc no devolvió la página de pago")
	}
	return &out, raw, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) ([]byte, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("fintoc sin secret key")
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", c.SecretKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return raw, fmt.Errorf("fintoc %s %s: %s", method, res.Status, clip(raw))
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return raw, err
		}
	}
	return raw, nil
}

func clip(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > 300 {
		return s[:300]
	}
	return s
}
