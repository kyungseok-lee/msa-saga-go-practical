.PHONY: help build up down logs clean test

help: ## 도움말 표시
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## 모든 서비스 빌드
	@echo "Building all services..."
	docker compose build

up: ## 모든 서비스 시작
	@echo "Starting all services..."
	docker compose up -d
	@echo "\n✅ All services started!"
	@echo "  - Kafka UI: http://localhost:8080"
	@echo "  - Temporal UI: http://localhost:8088"
	@echo "  - Order Service: http://localhost:8001/health"
	@echo "  - Payment Service: http://localhost:8002/health"
	@echo "  - Inventory Service: http://localhost:8003/health"
	@echo "  - Delivery Service: http://localhost:8004/health"

down: ## 모든 서비스 중지
	@echo "Stopping all services..."
	docker compose down

down-v: ## 모든 서비스 중지 및 볼륨 삭제
	@echo "Stopping all services and removing volumes..."
	docker compose down -v

logs: ## 전체 로그 확인
	docker compose logs -f

logs-order: ## Order Service 로그
	docker compose logs -f order-service

logs-payment: ## Payment Service 로그
	docker compose logs -f payment-service

logs-inventory: ## Inventory Service 로그
	docker compose logs -f inventory-service

logs-delivery: ## Delivery Service 로그
	docker compose logs -f delivery-service

restart-order: ## Order Service 재시작
	docker compose restart order-service

restart-payment: ## Payment Service 재시작
	docker compose restart payment-service

restart-inventory: ## Inventory Service 재시작
	docker compose restart inventory-service

restart-delivery: ## Delivery Service 재시작
	docker compose restart delivery-service

clean: ## 정리 (컨테이너, 볼륨, 네트워크)
	@echo "Cleaning up..."
	docker compose down -v --remove-orphans
	docker system prune -f

test: ## 테스트 실행
	@echo "Running tests..."
	go test -v ./...

test-coverage: ## 테스트 커버리지
	@echo "Running tests with coverage..."
	go test -cover -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

fmt: ## 코드 포맷팅
	@echo "Formatting code..."
	go fmt ./...

lint: ## 린트 실행
	@echo "Running linter..."
	golangci-lint run

create-order: ## 주문 생성 (예제)
	@echo "Creating order..."
	curl -X POST http://localhost:8001/orders \
		-H "Content-Type: application/json" \
		-d '{"userId":1001,"amount":50000,"quantity":1,"idempotencyKey":"order-test-001"}'

get-order: ## 주문 조회 (ORDER_ID 필요)
	@echo "Getting order (ID: ${ORDER_ID})..."
	curl http://localhost:8001/orders/${ORDER_ID}

check-kafka: ## Kafka 토픽 확인
	@echo "Kafka topics:"
	docker exec -it kafka kafka-topics.sh --list --bootstrap-server localhost:9092

check-db-order: ## Order DB 확인
	docker exec -it postgres-order psql -U order -d order_db -c "SELECT id, user_id, amount, status FROM orders ORDER BY created_at DESC LIMIT 10;"

check-db-payment: ## Payment DB 확인
	docker exec -it postgres-payment psql -U payment -d payment_db -c "SELECT id, order_id, amount, status FROM payments ORDER BY created_at DESC LIMIT 10;"

check-db-inventory: ## Inventory DB 확인
	docker exec -it postgres-inventory psql -U inventory -d inventory_db -c "SELECT product_id, product_name, available_quantity, reserved_quantity FROM inventory;"

check-redis: ## Redis 확인
	docker exec -it redis redis-cli KEYS "*"

ps: ## 실행 중인 서비스 확인
	docker compose ps

health: ## 모든 서비스 헬스 체크
	@echo "Checking service health..."
	@curl -s http://localhost:8001/health && echo " ✅ Order Service"
	@curl -s http://localhost:8002/health && echo " ✅ Payment Service"
	@curl -s http://localhost:8003/health && echo " ✅ Inventory Service"
	@curl -s http://localhost:8004/health && echo " ✅ Delivery Service"

