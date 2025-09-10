# Makefile di root repo
COMPOSE = docker compose -f deployments/docker-compose.yaml
KAFKA_BIN = /opt/bitnami/kafka/bin

.PHONY: up down ps logs kafka-shell redis-cli psql topics.producer topics.consumer

up:
	$(COMPOSE) up -d

down:
	$(COMPOSE) down -v

ps:
	$(COMPOSE) ps

logs:
	$(COMPOSE) logs -f --tail=200

kafka-shell:
	$(COMPOSE) exec kafka bash

redis-cli:
	$(COMPOSE) exec redis redis-cli

psql:
	$(COMPOSE) exec postgres psql -U app -d orders

# Helper testing Kafka (console)
topics.producer:
	$(COMPOSE) exec -T kafka bash -lc '$(KAFKA_BIN)/kafka-console-producer.sh --bootstrap-server kafka:9092 --topic order.created'

topics.consumer:
	$(COMPOSE) exec -T kafka bash -lc '$(KAFKA_BIN)/kafka-console-consumer.sh --bootstrap-server kafka:9092 --topic order.created --from-beginning'

kafka-shell:
	$(COMPOSE) exec kafka bash

topics.create:
	$(COMPOSE) exec -T kafka bash -lc '$(KAFKA_BIN)/kafka-topics.sh --create --topic order.created --partitions 3 --replication-factor 1 --bootstrap-server kafka:9092'

topics.list:
	$(COMPOSE) exec -T kafka bash -lc '$(KAFKA_BIN)/kafka-topics.sh --list --bootstrap-server kafka:9092'
