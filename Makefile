.PHONY: run build docker-up docker-down test lint

run:
	go run ./cmd/server/main.go

build:
	go build -o server ./cmd/server/main.go

test:
	go test ./...

lint:
	golangci-lint run ./...

docker-up:
	docker compose up --build

docker-down:
	docker compose down -v

