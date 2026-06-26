DATABASE_URL ?= postgres://notify:notify@localhost:5432/notify?sslmode=disable

.PHONY: up down migrate test test-integration lint tidy

up:
	docker compose up -d --build

down:
	docker compose down -v

migrate:
	go run github.com/pressly/goose/v3/cmd/goose@latest -dir migrations postgres "$(DATABASE_URL)" up

test:
	go test ./...

# Integration suite (real Postgres + Mailpit via testcontainers, real Temporal
# dev server). Requires a running container engine. Excluded from `make test`.
test-integration:
	go test -tags=integration -timeout=300s ./test/integration/...

lint:
	golangci-lint run

tidy:
	go mod tidy
