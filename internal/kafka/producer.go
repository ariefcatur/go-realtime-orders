package kafka

import (
	"context"
	"github.com/segmentio/kafka-go"
	"time"
)

type Producer struct {
	w       *kafka.Writer
	inbox   chan kafka.Message
	closeCh chan struct{}
}

func NewProducer(brokers []string, topic string, buf int) *Producer {
	return &Producer{
		w: &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        topic,
			Balancer:     &kafka.Hash{},
			RequiredAcks: kafka.RequireAll,
			Async:        true, // fire-and-forget untuk throughput; log error di loop
		},
		inbox:   make(chan kafka.Message, buf),
		closeCh: make(chan struct{}),
	}
}

func (p *Producer) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(p.inbox)
				for m := range p.inbox {
					_ = p.w.WriteMessages(context.Background(), m)
				}
				_ = p.w.Close()
				close(p.closeCh)
				return
			case m, ok := <-p.inbox:
				if !ok {
					_ = p.w.Close()
					return
				}
				_ = p.w.WriteMessages(context.Background(), m) // TODO: handle/log error
			}
		}
	}()
}

func (p *Producer) Publish(key, value []byte, headers ...kafka.Header) {
	p.inbox <- kafka.Message{
		Key:     key,
		Value:   value,
		Time:    time.Now(),
		Headers: headers,
	}
}

// Tutup channel supaya goroutine nge-flush sisa pesan lalu exit rapi.
func (p *Producer) Close() { close(p.inbox) }

// Tunggu sampai goroutine selesai.
func (p *Producer) WaitClosed() { <-p.closeCh }
