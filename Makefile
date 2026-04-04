APP_NAME := vaultsend-api
DB_URL ?= postgres://vaultsend:vaultsend@localhost:5432/vaultsend?sslmode=disable

.PHONY: run test lint migrate-up migrate-down sqlc-generate

run:
	go run ./cmd/api

test:
	go test ./...

lint:
	go vet ./...

migrate-up:
	migrate -path db/migrations -database "$(DB_URL)" up

migrate-down:
	migrate -path db/migrations -database "$(DB_URL)" down 1

sqlc-generate:
	sqlc generate
