package fintoc

import (
	"encoding/json"
	"strings"
)

// Decision es lo que el aviso pide hacer con el cobro. Ignore no lo toca.
type Decision struct {
	Action    string
	SessionID string
	BuyOrder  string
	Amount    int64
	Currency  string
	PaymentID string
}

func Interpret(raw []byte) (Decision, error) {
	var notice struct {
		Type string `json:"type"`
		Data struct {
			ID                string            `json:"id"`
			Object            string            `json:"object"`
			Status            string            `json:"status"`
			Amount            int64             `json:"amount"`
			Currency          string            `json:"currency"`
			Metadata          map[string]string `json:"metadata"`
			CheckoutSessionID string            `json:"checkout_session_id"`
			PaymentType       string            `json:"payment_type"`
			PaymentResource   struct {
				PaymentIntent *struct {
					ID          string            `json:"id"`
					Status      string            `json:"status"`
					Amount      int64             `json:"amount"`
					Currency    string            `json:"currency"`
					PaymentType string            `json:"payment_type"`
					Metadata    map[string]string `json:"metadata"`
				} `json:"payment_intent"`
			} `json:"payment_resource"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &notice); err != nil {
		return Decision{}, err
	}
	data := notice.Data
	object := strings.TrimSpace(data.Object)
	eventType := strings.TrimSpace(notice.Type)
	if object == "" {
		switch {
		case strings.HasPrefix(eventType, "checkout_session"):
			object = "checkout_session"
		case strings.HasPrefix(eventType, "payment_intent"):
			object = "payment_intent"
		}
	}

	out := Decision{Action: "ignore", Currency: strings.TrimSpace(data.Currency), Amount: data.Amount}
	out.BuyOrder = strings.TrimSpace(data.Metadata["buy_order"])
	switch object {
	case "checkout_session":
		out.SessionID = strings.TrimSpace(data.ID)
	case "payment_intent":
		out.SessionID = strings.TrimSpace(data.CheckoutSessionID)
		out.PaymentID = strings.TrimSpace(data.ID)
		if data.PaymentType != "" && !strings.EqualFold(data.PaymentType, "bank_transfer") {
			out.Action = "mismatch"
			return out, nil
		}
	default:
		return out, nil
	}

	status := strings.ToLower(strings.TrimSpace(data.Status))
	if pi := data.PaymentResource.PaymentIntent; pi != nil {
		out.PaymentID = strings.TrimSpace(pi.ID)
		if pi.Amount > 0 {
			if out.Amount > 0 && pi.Amount != out.Amount {
				out.Action = "mismatch"
				return out, nil
			}
			out.Amount = pi.Amount
		}
		if strings.TrimSpace(pi.Currency) != "" {
			out.Currency = strings.TrimSpace(pi.Currency)
		}
		if order := strings.TrimSpace(pi.Metadata["buy_order"]); order != "" {
			out.BuyOrder = order
		}
		if pi.PaymentType != "" && !strings.EqualFold(pi.PaymentType, "bank_transfer") {
			out.Action = "mismatch"
			return out, nil
		}
		if strings.TrimSpace(pi.Status) != "" {
			status = strings.ToLower(strings.TrimSpace(pi.Status))
		}
	}

	switch {
	case eventType == "payment_intent.succeeded" || status == "succeeded":
		out.Action = "pay"
	case eventType == "payment_intent.failed" || eventType == "checkout_session.expired" || terminal(status):
		out.Action = "fail"
	default:
		out.Action = "ignore"
	}
	return out, nil
}

func terminal(status string) bool {
	switch status {
	case "failed", "rejected", "expired", "canceled", "cancelled":
		return true
	default:
		return false
	}
}
