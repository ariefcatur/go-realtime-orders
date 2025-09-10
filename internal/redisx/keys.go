package redisx

import "time"

const (
	// Idempotency create order: idem:order:create:{external_id} -> order_id
	KeyIdemOrderCreate = "idem:order:create:%s"

	// Cache status order: order_status:{order_id} -> {"status": "...", "updated_at": "..."}
	KeyOrderStatus = "order_status:%s"

	// Dedup event processing: dedup:{service}:{id} (id = event_id atau order_id:phase)
	KeyDedup = "dedup:%s:%s"

	// Saga state per order: hash saga:{order_id}
	KeySaga = "saga:%s"
)

var (
	TTLIdempotency = 24 * time.Hour
	TTLStatusCache = 5 * time.Minute
	TTLDedup       = 48 * time.Hour
	TTLSaga        = 48 * time.Hour
)
