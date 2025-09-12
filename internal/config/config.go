package config

import (
	"os"
	"strings"
)

type Config struct {
	HTTPAddr     string
	PostgresDSN  string
	RedisAddr    string
	KafkaBrokers []string
	ServiceName  string
}

func Load() Config {
	return Config{
		HTTPAddr:     getenv("HTTP_ADDR", ":8081"),
		PostgresDSN:  getenv("POSTGRES_DSN", "postgres://app:secret@postgres:5432/orders?sslmode=disable"),
		RedisAddr:    getenv("REDIS_ADDR", "redis:6379"),
		KafkaBrokers: splitCSV(getenv("KAFKA_BROKERS", "kafka:9092")),
		ServiceName:  getenv("SERVICE_NAME", "order-api"),
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
