package orders

import (
	"encoding/json"
	"time"
)

const (
	EventOrderCreated      = "OrderCreated"
	EventStockReserved     = "StockReserved"
	EventStockRejected     = "StockRejected"
	EventPaymentAuthorized = "PaymentAuthorized"
	EventPaymentFailed     = "PaymentFailed"
	EventOrderFinalized    = "OrderFinalized"
)

type Envelope struct {
	EventID       string          `json:"event_id"`      // uuid
	EventType     string          `json:"event_type"`    // salah satu const di atas
	EventVersion  int             `json:"event_version"` // 1
	OccurredAt    time.Time       `json:"occurred_at"`   // RFC3339
	Producer      string          `json:"producer"`      // e.g., "order-api"
	TraceID       string          `json:"trace_id,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"` // biasanya order_id
	Payload       json.RawMessage `json:"payload"`                  // payload spesifik
}

// ---- Payload tipe per event ----

type ItemQty struct {
	ProductID string `json:"product_id"`
	Qty       int    `json:"qty"`
}

type ItemPrice struct {
	ProductID  string `json:"product_id"`
	Qty        int    `json:"qty"`
	PriceCents int    `json:"price_cents"`
}

type OrderCreatedPayload struct {
	OrderID    string      `json:"order_id"`
	ExternalID string      `json:"external_id"`
	UserID     string      `json:"user_id"`
	Items      []ItemPrice `json:"items"`
	TotalCents int         `json:"total_cents"`
}

type StockReservedPayload struct {
	OrderID string    `json:"order_id"`
	Items   []ItemQty `json:"items"`
}

type StockRejectedDetail struct {
	ProductID string `json:"product_id"`
	Required  int    `json:"required"`
	Available int    `json:"available"`
}

type StockRejectedPayload struct {
	OrderID string                `json:"order_id"`
	Reason  string                `json:"reason"` // e.g., OUT_OF_STOCK
	Details []StockRejectedDetail `json:"details,omitempty"`
}

type PaymentAuthorizedPayload struct {
	OrderID     string `json:"order_id"`
	PaymentRef  string `json:"payment_ref"`
	AmountCents int    `json:"amount_cents"`
}

type PaymentFailedPayload struct {
	OrderID string `json:"order_id"`
	Reason  string `json:"reason"` // e.g., INSUFFICIENT_FUNDS
}

type OrderFinalizedPayload struct {
	OrderID     string   `json:"order_id"`
	FinalStatus string   `json:"final_status"`      // COMPLETED | FAILED
	Reasons     []string `json:"reasons,omitempty"` // jika FAILED
}
