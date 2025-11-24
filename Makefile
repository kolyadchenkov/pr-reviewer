.PHONY: run build docker-up docker-down test

run:
	go run ./...

build:
	go build -o server main.go

test:
	go test ./...

docker-up:
	docker compose up --build

docker-down:
	docker compose down -v

