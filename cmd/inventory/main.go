package main

import (
	"context"
	"github.com/ariefcatur/go-realtime-orders.git/internal/config"
	"github.com/ariefcatur/go-realtime-orders.git/internal/inventory"
	kafkax "github.com/ariefcatur/go-realtime-orders.git/internal/kafka"
	"github.com/ariefcatur/go-realtime-orders.git/internal/orders"
	"github.com/ariefcatur/go-realtime-orders.git/internal/postgres"
	"github.com/ariefcatur/go-realtime-orders.git/internal/redisx"
	"github.com/joho/godotenv"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func mustAtoi(s, def string) int {
	if s == "" {
		s = def
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return 1
	}
	return i
}

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// DB
	db, err := postgres.Connect(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	// Redis
	rdb := redisx.New(cfg.RedisAddr)
	defer rdb.Close()

	// Producers: reserved & rejected (dua topic berbeda)
	pOK := kafkax.NewProducer(cfg.KafkaBrokers, orders.TopicStockReserved, 1024)
	pOK.Start(ctx)
	pRJ := kafkax.NewProducer(cfg.KafkaBrokers, orders.TopicStockRejected, 1024)
	pRJ.Start(ctx)

	// Service
	svc := &inventory.Service{
		Repo:           &orders.ReservationRepo{DB: db},
		Redis:          rdb,
		ProducerOK:     pOK,
		ProducerReject: pRJ,
		ServiceName:    cfg.ServiceName + "-inventory",
	}

	// Consumer
	group := getenv("INVENTORY_GROUP", "inventory-svc")
	workers := mustAtoi(os.Getenv("INVENTORY_WORKERS"), "8")
	cons := kafkax.NewConsumer(cfg.KafkaBrokers, group, orders.TopicOrderCreated, workers)

	go func() {
		log.Printf("inventory consumer started: group=%s topic=%s workers=%d", group, orders.TopicOrderCreated, workers)
		if err := cons.Start(ctx, svc.HandleOrderCreated); err != nil {
			log.Printf("consumer exit: %v", err)
			cancel()
		}
	}()

	// graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down consumer...")
	cancel()
	time.Sleep(500 * time.Millisecond)
	pOK.Close()
	pRJ.Close()
	pOK.WaitClosed()
	pRJ.WaitClosed()
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
