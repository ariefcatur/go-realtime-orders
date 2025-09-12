package main

import (
	"context"
	"github.com/ariefcatur/go-realtime-orders.git/internal/config"
	"github.com/ariefcatur/go-realtime-orders.git/internal/httpx"
	kafkax "github.com/ariefcatur/go-realtime-orders.git/internal/kafka"
	"github.com/ariefcatur/go-realtime-orders.git/internal/orders"
	"github.com/ariefcatur/go-realtime-orders.git/internal/postgres"
	"github.com/ariefcatur/go-realtime-orders.git/internal/redisx"
	"github.com/joho/godotenv"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// DB
	db, err := postgres.Connect(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer db.Close()

	// Redis
	rdb := redisx.New(cfg.RedisAddr)
	defer rdb.Close()

	// Kafka producer
	prod := kafkax.NewProducer(cfg.KafkaBrokers, orders.TopicOrderCreated, 1024)
	prod.Start(ctx)

	// Repo & handler
	repo := &orders.Repo{DB: db}
	router := httpx.NewRouter()
	oh := &httpx.OrdersHandler{
		Repo:     repo,
		Producer: prod,
		Redis:    rdb,
		Service:  cfg.ServiceName,
	}
	oh.Register(router)

	// HTTP server
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: router}

	// graceful shutdown
	go func() {
		log.Printf("HTTP listening at %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// wait signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down...")

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	_ = srv.Shutdown(ctx2)
	prod.Close()      // tutup inbox -> flush & close writer
	cancel()          // stop producer loop
	prod.WaitClosed() // drain
}
