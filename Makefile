.PHONY: run seed seed-compose fmt test vet build up down logs

run:
	go run main.go

seed:
	go run ./cmd/seed

seed-compose:
	docker compose run --rm seed

fmt:
	gofmt -w .

test:
	go test ./...

vet:
	go vet ./...

build:
	mkdir -p bin
	go build -o bin/ecommerce-api .

up:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f api
