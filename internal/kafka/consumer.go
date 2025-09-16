package kafka

import (
	"context"
	"github.com/segmentio/kafka-go"
	"log"
	"time"
)

// Handler harus return nil hanya jika proses sukses & boleh commit offset.
type Handler func(ctx context.Context, m kafka.Message) error

type Consumer struct {
	r       *kafka.Reader
	workers int
}

func NewConsumer(brokers []string, group, topic string, workers int) *Consumer {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers,
		GroupID:        group,
		Topic:          topic,
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: 0, // manual commit
	})
	if workers <= 0 {
		workers = 1
	}
	return &Consumer{r: r, workers: workers}
}

func (c *Consumer) Start(ctx context.Context, h Handler) error {
	defer c.r.Close()

	jobs := make(chan kafka.Message, 1024)
	errs := make(chan error, c.workers)

	// workers
	for i := 0; i < c.workers; i++ {
		go func(id int) {
			for m := range jobs {
				if err := h(ctx, m); err != nil {
					errs <- err
					continue
				}
				// commit on success
				if err := c.r.CommitMessages(ctx, m); err != nil {
					errs <- err
				}
			}
		}(i)
	}

	// dispatcher loop
	for {
		m, err := c.r.ReadMessage(ctx)
		if err != nil {
			close(jobs)
			// kecilkan noise saat shutdown
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		select {
		case jobs <- m:
		case <-ctx.Done():
			close(jobs)
			return nil
		}

		// non-blocking drain error agar tidak deadlock
		select {
		case e := <-errs:
			log.Printf("worker error: %v", e)
			time.Sleep(200 * time.Millisecond) // backoff ringan
		default:
		}
	}
}
