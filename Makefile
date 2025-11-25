.PHONY: run build docker-up docker-down test lint migrate-up migrate-down migrate-create

run:
	go run ./cmd/server/main.go

build:
	go build -o server ./cmd/server/main.go

test:
	go test ./... -v

lint:
	golangci-lint run ./...

docker-up:
	docker compose up --build

docker-down:
	docker compose down -v

migrate-up:
	go run -tags='no_embed' github.com/pressly/goose/v3/cmd/goose -dir migrations postgres "$${DATABASE_URL:-postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable}" up

migrate-down:
	go run -tags='no_embed' github.com/pressly/goose/v3/cmd/goose -dir migrations postgres "$${DATABASE_URL:-postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable}" down

migrate-create:
	@if [ -z "$(NAME)" ]; then \
		echo "Usage: make migrate-create NAME=<migration_name>"; \
		exit 1; \
	fi
	go run -tags='no_embed' github.com/pressly/goose/v3/cmd/goose -dir migrations create $(NAME) sql

