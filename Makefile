# ===== Config =====
COMPOSE = docker compose -f deployments/docker-compose.yaml

# Load .env (jika ada) agar semua target otomatis pakai env yang sama
ifneq (,$(wildcard .env))
include .env
export
endif

# ===== Help =====
.PHONY: help
help:
	@echo "Targets:"
	@echo "  make dev        -> Up infra + migrate DB + run API (host)"
	@echo "  make up         -> Start infra (Kafka, Redis, Postgres, UI)"
	@echo "  make down       -> Stop infra & remove volumes"
	@echo "  make migrate    -> Apply SQL migrations (000, 001, 010)"
	@echo "  make api        -> Run API (go run ./cmd/api)"
	@echo "  make ps         -> Show container status"
	@echo "  make logs       -> Tail compose logs"
	@echo "  make products   -> Quick SELECT products via psql"
	@echo "  make demo-sku   -> Demo create order by SKU (curl)"
	@echo "  make consume    -> Console consumer topic order.created"
	@echo "  make produce    -> Console producer topic order.created"

# ===== Infra =====
.PHONY: up down ps logs
up:
	$(COMPOSE) up -d

down:
	$(COMPOSE) down -v

ps:
	$(COMPOSE) ps

logs:
	$(COMPOSE) logs -f --tail=200

# ===== DB / Migrations =====
.PHONY: psql migrate products reset-db
psql:
	$(COMPOSE) exec postgres psql -U app -d orders

migrate:
	@cat db/migrations/000_init.sql      | $(COMPOSE) exec -T postgres psql -U app -d orders -v ON_ERROR_STOP=1 -f -
	@cat db/migrations/001_triggers.sql  | $(COMPOSE) exec -T postgres psql -U app -d orders -v ON_ERROR_STOP=1 -f -
	@cat db/migrations/010_seed.sql      | $(COMPOSE) exec -T postgres psql -U app -d orders -v ON_ERROR_STOP=1 -f -
	@echo "✅ migrations applied"

products:
	@echo "\pset border 2 \n SELECT id, sku, name, stock, price_cents FROM products ORDER BY sku;" \
	| $(COMPOSE) exec -T postgres psql -U app -d orders

reset-db:
	@echo "DROP SCHEMA public CASCADE; CREATE SCHEMA public;" \
	| $(COMPOSE) exec -T postgres psql -U app -d orders

# ===== API =====
.PHONY: api dev
api:
	go run ./cmd/api

dev: up migrate api

inventory:
	go run ./cmd/inventory

dev-inventory:
	$(MAKE) up
	$(MAKE) inventory

# ===== Kafka console tools =====
.PHONY: kafka-shell consume produce
kafka-shell:
	$(COMPOSE) exec kafka bash

consume:
	$(COMPOSE) exec -T kafka bash -lc \
	'kafka-console-consumer.sh --bootstrap-server kafka:9092 --topic order.created --from-beginning'

produce:
	$(COMPOSE) exec -T kafka bash -lc \
	'kafka-console-producer.sh --bootstrap-server kafka:9092 --topic order.created'

# ===== Demo =====
.PHONY: demo-sku
demo-sku:
	@echo "Hit /products to see IDs/SKUs:"
	@curl -s http://localhost:8081/products | jq '.[] | {id, sku, name, stock, price_cents}' || true
	@echo "\nCreating order by SKU..."
	@curl -s -X POST 'http://localhost:8081/orders/sku' \
	  -H 'Content-Type: application/json' \
	  -d '{ "external_id":"EXT-$$RANDOM", "user_id":"00000000-0000-0000-0000-000000000001",
	        "items":[ {"sku":"SKU-APPLE","qty":2}, {"sku":"SKU-RICE","qty":1} ] }' \
	  | jq .
