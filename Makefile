COMPOSE ?= docker compose
COMPOSE_FILE ?= docker-compose.yml

.PHONY: docker-build docker-run docker-stop

docker-build:
	$(COMPOSE) -f $(COMPOSE_FILE) build

docker-run:
	$(COMPOSE) -f $(COMPOSE_FILE) up --build

docker-stop:
	$(COMPOSE) -f $(COMPOSE_FILE) down
