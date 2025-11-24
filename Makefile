.PHONY: run build docker-up docker-down test lint

run:
	go run ./...

build:
	go build -o server main.go

test:
	go test ./...

lint:
	golangci-lint run ./...

docker-up:
	docker compose up --build

docker-down:
	docker compose down -v

