.PHONY: build run test clean docker-build docker-up docker-down

build:
	go build -o bin/wallet-service cmd/main.go

run:
	go run cmd/main.go

test:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

docker-build:
	docker-compose build

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down -v

docker-logs:
	docker-compose logs -f

migrate:
	docker-compose exec postgres psql -U postgres -d walletdb -f /docker-entrypoint-initdb.d/001_create_wallets_table.sql

benchmark:
	hey -n 1000 -c 10 -m POST -H "Content-Type: application/json" -d '{"walletId":"$(WALLET_ID)","operationType":"DEPOSIT","amount":100}' http://localhost:8080/api/v1/wallets/transaction