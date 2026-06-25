DATABASE_URL ?= postgres://notify:notify@localhost:5432/notify?sslmode=disable

.PHONY: up down migrate test lint tidy

up:
	docker compose up -d --build

down:
	docker compose down -v

migrate:
	go run github.com/pressly/goose/v3/cmd/goose@latest -dir migrations postgres "$(DATABASE_URL)" up

test:
	go test ./...

lint:
	golangci-lint run

tidy:
	go mod tidy
