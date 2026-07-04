test: 
	go test -v tests/index_test.go

build:
	go build -o bin/wallet-service cmd/main.go

run:
	go run cmd/main.go

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