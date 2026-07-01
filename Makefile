COMPOSE ?= docker compose
COMPOSE_FILE ?= docker-compose.yml

.PHONY: test e2e docker-build docker-run docker-stop

test:
	go test ./...

e2e:
	.github/scripts/e2e.sh

docker-build:
	$(COMPOSE) -f $(COMPOSE_FILE) build

docker-run:
	$(COMPOSE) -f $(COMPOSE_FILE) up --build

docker-stop:
	$(COMPOSE) -f $(COMPOSE_FILE) down
