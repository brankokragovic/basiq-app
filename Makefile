-include .env
export

.PHONY: api build clean

api: redis
	go run ./cmd/api

build:
	docker compose build

clean:
	docker compose down -v

redis:
	@docker compose up -d redis
	@echo "waiting for redis..."
	@until docker compose exec redis redis-cli ping 2>/dev/null | grep -q PONG; do sleep 1; done
