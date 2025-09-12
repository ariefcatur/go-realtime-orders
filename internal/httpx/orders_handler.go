package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	kafkax "github.com/ariefcatur/go-realtime-orders.git/internal/kafka"
	"github.com/ariefcatur/go-realtime-orders.git/internal/orders"
	"github.com/ariefcatur/go-realtime-orders.git/internal/redisx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	kafkago "github.com/segmentio/kafka-go"
	"net/http"
	"time"
)

type CreateOrderBySKUReq struct {
	ExternalID string                `json:"external_id"`
	UserID     string                `json:"user_id"`
	Items      []orders.ItemInputSKU `json:"items"`
}

type OrdersHandler struct {
	Repo     *orders.Repo
	Producer *kafkax.Producer
	Redis    *redis.Client
	Service  string
}

type CreateOrderReq struct {
	ExternalID string             `json:"external_id"`
	UserID     string             `json:"user_id"`
	Items      []orders.ItemInput `json:"items"`
}

type CreateOrderResp struct {
	OrderID    string `json:"order_id"`
	TotalCents int    `json:"total_cents"`
	Idempotent bool   `json:"idempotent"`
}

func (h *OrdersHandler) Register(r *chi.Mux) {
	r.Post("/orders", h.createOrder)
	r.Post("/orders/sku", h.createOrderBySKU)
	r.Get("/orders/{id}", h.getOrder)
	r.Get("/products", h.listProducts)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *OrdersHandler) createOrderBySKU(w http.ResponseWriter, r *http.Request) {
	var req CreateOrderBySKUReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.ExternalID == "" || req.UserID == "" || len(req.Items) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing fields"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	orderID, total, existed, err := h.Repo.CreateOrderBySKU(ctx, req.ExternalID, req.UserID, req.Items)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// set idempotency & cache status seperti createOrder()
	idemKey := fmt.Sprintf(redisx.KeyIdemOrderCreate, req.ExternalID)
	_ = h.Redis.Set(ctx, idemKey, orderID, redisx.TTLIdempotency).Err()
	statusKey := fmt.Sprintf(redisx.KeyOrderStatus, orderID)
	_ = h.Redis.Set(ctx, statusKey, `{"status":"CREATED"}`, redisx.TTLStatusCache).Err()

	// publish event sama persis (OrderCreated)
	ev := orders.Envelope{
		EventID:       uuid.NewString(),
		EventType:     orders.EventOrderCreated,
		EventVersion:  1,
		OccurredAt:    time.Now().UTC(),
		Producer:      h.Service,
		TraceID:       r.Header.Get("X-Request-Id"),
		CorrelationID: orderID,
	}
	// payload items bisa diisi qty + biar ringan, price 0 (downstream bisa re-query)
	ev.Payload = kafkax.MustMarshal(orders.OrderCreatedPayload{
		OrderID:    orderID,
		ExternalID: req.ExternalID,
		UserID:     req.UserID,
		Items:      nil, // atau mapping SKU->ID kalau mau lengkap
		TotalCents: total,
	})
	h.Producer.Publish(orders.PartitionKey(orderID), kafkax.MustMarshal(ev),
		kafkago.Header{Key: "x-event-type", Value: []byte(orders.EventOrderCreated)},
		kafkago.Header{Key: "x-event-version", Value: []byte("1")},
	)

	writeJSON(w, http.StatusAccepted, CreateOrderResp{OrderID: orderID, TotalCents: total, Idempotent: existed})
}

func (h *OrdersHandler) listProducts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	ps, err := h.Repo.ListProducts(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

func (h *OrdersHandler) createOrder(w http.ResponseWriter, r *http.Request) {
	var req CreateOrderReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.ExternalID == "" || req.UserID == "" || len(req.Items) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing fields"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Fast-path idempotency via Redis (optional, DB tetap jadi kebenaran)
	idemKey := fmt.Sprintf(redisx.KeyIdemOrderCreate, req.ExternalID)
	if ok, _ := redisx.Exists(ctx, h.Redis, idemKey); ok {
		// fall back ke DB buat ambil nilai akurat
		// (di demo, kita langsung ke DB lewat repo CreateOrderTx yang handle existed)
	}

	orderID, total, existed, err := h.Repo.CreateOrderTx(ctx, req.ExternalID, req.UserID, req.Items)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Simpan shortcut idempotency di Redis (TTL 24h)
	_ = h.Redis.Set(ctx, idemKey, orderID, redisx.TTLIdempotency).Err()

	// Cache status (CREATED) agar GET cepat
	statusKey := fmt.Sprintf(redisx.KeyOrderStatus, orderID)
	_ = h.Redis.Set(ctx, statusKey, `{"status":"CREATED"}`, redisx.TTLStatusCache).Err()

	// Publish event (envelope v1)
	ev := orders.Envelope{
		EventID:       uuid.NewString(),
		EventType:     orders.EventOrderCreated,
		EventVersion:  1,
		OccurredAt:    time.Now().UTC(),
		Producer:      h.Service,
		TraceID:       r.Header.Get("X-Request-Id"),
		CorrelationID: orderID,
	}
	payload := orders.OrderCreatedPayload{
		OrderID:    orderID,
		ExternalID: req.ExternalID,
		UserID:     req.UserID,
		Items:      toItemPrices(req.Items), // price diisi oleh consumer? Di sini cukup qty+id; tapi karena kita sudah hitung total dari DB, kita isi price di payload biar lengkap.
		TotalCents: total,
	}
	ev.Payload = kafkax.MustMarshal(payload)
	value := kafkax.MustMarshal(ev)

	h.Producer.Publish(
		orders.PartitionKey(orderID),
		value,
		kafkago.Header{Key: "x-event-type", Value: []byte(orders.EventOrderCreated)},
		kafkago.Header{Key: "x-event-version", Value: []byte("1")},
	)

	writeJSON(w, http.StatusAccepted, CreateOrderResp{OrderID: orderID, TotalCents: total, Idempotent: existed})
}

func toItemPrices(items []orders.ItemInput) []orders.ItemPrice {
	out := make([]orders.ItemPrice, 0, len(items))
	for _, it := range items {
		out = append(out, orders.ItemPrice{ProductID: it.ProductID, Qty: it.Qty, PriceCents: 0})
	}
	return out
}

func (h *OrdersHandler) getOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "id")
	if orderID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	// 1) coba cache
	key := fmt.Sprintf(redisx.KeyOrderStatus, orderID)
	if s, err := h.Redis.Get(ctx, key).Result(); err == nil && s != "" {
		writeJSON(w, http.StatusOK, json.RawMessage(s))
		return
	}

	// 2) fallback DB
	status, err := h.Repo.GetOrderStatus(ctx, orderID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	body := map[string]any{"status": status}
	b, _ := json.Marshal(body)
	_ = h.Redis.Set(ctx, key, b, redisx.TTLStatusCache).Err()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}
