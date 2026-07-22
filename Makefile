APP_NAME := vaultsend-api
DB_URL ?= postgres://vaultsend:vaultsend@localhost:5432/vaultsend?sslmode=disable

.PHONY: run run-worker run-cleanup-worker web-install web-run web-lint web-typecheck web-build test lint migrate-up migrate-down sqlc-generate

run:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

run-cleanup-worker:
	go run ./cmd/cleanup-worker

web-install:
	cd web && npm install

web-run:
	cd web && npm run dev

web-lint:
	cd web && npm run lint

web-typecheck:
	cd web && npm run typecheck

web-build:
	cd web && npm run build

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
