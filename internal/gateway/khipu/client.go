// Package khipu habla con la API 3.0 de pagos instantáneos.
// La misma URL sirve para una cuenta de desarrollador y para una de producción.
package khipu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://payment-api.khipu.com"

type Client struct {
	APIKey string
	HTTP   *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		APIKey: strings.TrimSpace(apiKey),
		HTTP:   &http.Client{Timeout: 25 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.APIKey != ""
}

type CreateRequest struct {
	Amount           float64 `json:"amount"`
	Currency         string  `json:"currency"`
	Subject          string  `json:"subject"`
	TransactionID    string  `json:"transaction_id"`
	ReturnURL        string  `json:"return_url"`
	CancelURL        string  `json:"cancel_url"`
	NotifyURL        string  `json:"notify_url,omitempty"`
	NotifyAPIVersion string  `json:"notify_api_version,omitempty"`
}

type CreateResponse struct {
	PaymentID  string `json:"payment_id"`
	PaymentURL string `json:"payment_url"`
}

type Payment struct {
	PaymentID               string     `json:"payment_id"`
	Status                  string     `json:"status"`
	Amount                  flexAmount `json:"amount"`
	Currency                string     `json:"currency"`
	TransactionID           string     `json:"transaction_id"`
	ConciliationDate        string     `json:"conciliation_date"`
	AuthorizerOperationCode string     `json:"authorizer_operation_code"`
	PaymentMethod           string     `json:"payment_method"`
}

// Reconciled es el pago que Khipu ya concilió. Volver del banco no alcanza.
func (p *Payment) Reconciled() bool {
	return p != nil && strings.TrimSpace(p.ConciliationDate) != ""
}

// Terminal es un cobro que ya no se puede pagar.
func (p *Payment) Terminal() bool {
	if p == nil || p.Reconciled() {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(p.Status)) {
	case "rejected", "expired", "canceled", "cancelled", "failed":
		return true
	default:
		return false
	}
}

func (c *Client) Create(ctx context.Context, req CreateRequest) (*CreateResponse, []byte, error) {
	var out CreateResponse
	raw, err := c.do(ctx, http.MethodPost, "/v3/payments", req, &out)
	if err != nil {
		return nil, raw, err
	}
	if strings.TrimSpace(out.PaymentID) == "" || strings.TrimSpace(out.PaymentURL) == "" {
		return nil, raw, fmt.Errorf("khipu no devolvió la página de pago")
	}
	return &out, raw, nil
}

func (c *Client) Get(ctx context.Context, paymentID string) (*Payment, []byte, error) {
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return nil, nil, fmt.Errorf("pago de khipu vacío")
	}
	var out Payment
	path := "/v3/payments/" + url.PathEscape(paymentID)
	raw, err := c.do(ctx, http.MethodGet, path, nil, &out)
	if err != nil {
		return nil, raw, err
	}
	return &out, raw, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) ([]byte, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("khipu sin API key")
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
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("x-api-key", c.APIKey)

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
		return raw, fmt.Errorf("khipu %s %s: %s", method, res.Status, clip(raw))
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

type flexAmount float64

func (a *flexAmount) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if strings.TrimSpace(s) == "" {
			return nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		*a = flexAmount(f)
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	*a = flexAmount(f)
	return nil
}
