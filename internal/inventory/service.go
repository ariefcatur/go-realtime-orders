package inventory

import (
	"context"
	"encoding/json"
	"fmt"
	kafkax "github.com/ariefcatur/go-realtime-orders.git/internal/kafka"
	"github.com/ariefcatur/go-realtime-orders.git/internal/orders"
	"github.com/ariefcatur/go-realtime-orders.git/internal/redisx"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	kafkago "github.com/segmentio/kafka-go"
	"time"
)

type Service struct {
	Repo           *orders.ReservationRepo
	Redis          *redis.Client
	ProducerOK     *kafkax.Producer // publish stock.reserved
	ProducerReject *kafkax.Producer // publish stock.rejected
	ServiceName    string
}

// HandleOrderCreated: dipasang sebagai handler consumer.
func (s *Service) HandleOrderCreated(ctx context.Context, m kafkago.Message) error {
	// 1) decode envelope
	var env orders.Envelope
	if err := json.Unmarshal(m.Value, &env); err != nil {
		return err
	}
	if env.EventType != orders.EventOrderCreated {
		return nil
	} // ignore

	// 2) dedup via Redis (pakai event_id)
	dkey := fmt.Sprintf(redisx.KeyDedup, "inventory", env.EventID)
	exists, _ := redisx.Exists(ctx, s.Redis, dkey)
	if exists {
		return nil
	}
	_ = s.Redis.Set(ctx, dkey, "1", redisx.TTLDedup).Err()

	// 3) decode payload
	var p orders.OrderCreatedPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return err
	}

	// Siapkan daftar item qty (abaikan price)
	items := make([]orders.ItemQty, 0, len(p.Items))
	for _, it := range p.Items {
		items = append(items, orders.ItemQty{ProductID: it.ProductID, Qty: it.Qty})
	}

	// 4) idempotent short-circuit: kalau sudah di-reserve sebelumnya
	if ok, _ := s.Repo.SudahReserved(ctx, p.OrderID, len(items)); ok {
		// publish reserved lagi (event ulang tidak masalah)
		return s.publishReserved(ctx, p.OrderID, items, env.TraceID)
	}

	// 5) coba reserve atomik
	ok, details, err := s.Repo.ReserveAll(ctx, p.OrderID, items)
	if err != nil {
		return err
	}

	if ok {
		return s.publishReserved(ctx, p.OrderID, items, env.TraceID)
	}
	// gagal stok → publish rejected (+details)
	return s.publishRejected(ctx, p.OrderID, details, env.TraceID)
}

func (s *Service) publishReserved(ctx context.Context, orderID string, items []orders.ItemQty, trace string) error {
	ev := orders.Envelope{
		EventID:       uuid.NewString(),
		EventType:     orders.EventStockReserved,
		EventVersion:  1,
		OccurredAt:    time.Now().UTC(),
		Producer:      s.ServiceName,
		TraceID:       trace,
		CorrelationID: orderID,
		Payload:       kafkax.MustMarshal(orders.StockReservedPayload{OrderID: orderID, Items: items}),
	}
	b := kafkax.MustMarshal(ev)
	s.ProducerOK.Publish(orders.PartitionKey(orderID), b,
		kafkago.Header{Key: "x-event-type", Value: []byte(orders.EventStockReserved)},
		kafkago.Header{Key: "x-event-version", Value: []byte("1")},
	)
	return nil
}

func (s *Service) publishRejected(ctx context.Context, orderID string, details []orders.StockRejectedDetail, trace string) error {
	ev := orders.Envelope{
		EventID:       uuid.NewString(),
		EventType:     orders.EventStockRejected,
		EventVersion:  1,
		OccurredAt:    time.Now().UTC(),
		Producer:      s.ServiceName,
		TraceID:       trace,
		CorrelationID: orderID,
		Payload: kafkax.MustMarshal(orders.StockRejectedPayload{
			OrderID: orderID, Reason: "OUT_OF_STOCK", Details: details,
		}),
	}
	b := kafkax.MustMarshal(ev)
	s.ProducerReject.Publish(orders.PartitionKey(orderID), b,
		kafkago.Header{Key: "x-event-type", Value: []byte(orders.EventStockRejected)},
		kafkago.Header{Key: "x-event-version", Value: []byte("1")},
	)
	return nil
}
