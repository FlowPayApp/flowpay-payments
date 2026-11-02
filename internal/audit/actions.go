package audit

const (
	ActionManualPayment    = "manual_payment_recorded"
	ActionTokenIssued      = "payment_token_issued"
	ActionCheckoutStarted  = "checkout_started"
	ActionLinkOpened       = "payment_link_opened"
	ActionPaymentCommitted = "payment_committed"
	ActionPaymentFailed    = "payment_failed"
	ActionWebhookAccepted  = "webhook_accepted"
	ActionWebhookRejected  = "webhook_rejected"
)
