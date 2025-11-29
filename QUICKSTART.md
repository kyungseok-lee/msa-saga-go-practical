# 빠른 시작 가이드 (5분)

이 가이드를 따라하면 5분 내에 MSA SAGA 패턴을 실행하고 테스트할 수 있습니다.

## 📋 준비물

- Docker & Docker Compose
- curl 또는 Postman

## 🚀 1단계: 서비스 시작 (2분)

```bash
# 프로젝트 클론
git clone <repository-url>
cd msa-saga-go-practical

# 모든 서비스 시작
docker compose up -d

# 서비스 시작 대기 (약 30초-1분)
# 로그 확인
docker compose logs -f
```

**확인 포인트:**
- ✅ 모든 PostgreSQL 인스턴스가 `ready to accept connections` 출력
- ✅ Kafka가 `started` 출력
- ✅ 각 서비스가 `http server starting` 출력

## ✅ 2단계: 헬스 체크 (30초)

```bash
# 모든 서비스 상태 확인
make health

# 또는 개별 확인
curl http://localhost:8001/health  # Order
curl http://localhost:8002/health  # Payment
curl http://localhost:8003/health  # Inventory
curl http://localhost:8004/health  # Delivery
```

**예상 응답:**
```json
{"status":"healthy"}
```

## 🎯 3단계: 성공 시나리오 테스트 (1분)

### 주문 생성

```bash
curl -X POST http://localhost:8001/orders \
  -H "Content-Type: application/json" \
  -d '{
    "userId": 1001,
    "amount": 50000,
    "quantity": 1,
    "idempotencyKey": "order-quickstart-001"
  }'
```

**응답 예시:**
```json
{
  "orderId": 1,
  "status": "PENDING"
}
```

### SAGA 플로우 확인 (약 1-2초 소요)

```bash
# 1초 대기
sleep 2

# 주문 상태 확인
curl http://localhost:8001/orders/1
```

**예상 최종 상태:**
```json
{
  "id": 1,
  "userId": 1001,
  "amount": 50000,
  "quantity": 1,
  "status": "COMPLETED",  ← 성공!
  "createdAt": "2025-01-29T10:00:00Z",
  "updatedAt": "2025-01-29T10:00:02Z"
}
```

### 이벤트 플로우 추적

```bash
# Order Service 로그
docker compose logs order-service | grep "order created successfully"

# Payment Service 로그
docker compose logs payment-service | grep "payment completed"

# Inventory Service 로그
docker compose logs inventory-service | grep "stock reserved"

# Delivery Service 로그
docker compose logs delivery-service | grep "delivery started"
```

## 🔥 4단계: 실패 시나리오 테스트 (1분)

### 재고 부족 시뮬레이션

```bash
# 재고 소진 (10개 주문)
for i in {1..10}; do
  curl -X POST http://localhost:8001/orders \
    -H "Content-Type: application/json" \
    -d "{
      \"userId\": 1001,
      \"amount\": 50000,
      \"quantity\": 50,
      \"idempotencyKey\": \"order-fail-test-$i\"
    }"
  sleep 0.2
done
```

### 보상 트랜잭션 확인

```bash
# Payment Service 로그에서 환불 확인
docker compose logs payment-service | grep "refund"

# 주문 상태 확인 (CANCELED 또는 FAILED 예상)
curl http://localhost:8001/orders/11
```

## 📊 5단계: UI로 모니터링 (1분)

### Kafka UI

1. 브라우저에서 http://localhost:8080 접속
2. Topics 메뉴 클릭
3. 다음 토픽들 확인:
   - `order.created.v1`
   - `payment.completed.v1`
   - `stock.reserved.v1`
   - `delivery.started.v1`

### DB 확인

```bash
# Order 테이블
docker exec -it postgres-order psql -U order -d order_db \
  -c "SELECT id, user_id, status FROM orders ORDER BY created_at DESC LIMIT 5;"

# Payment 테이블
docker exec -it postgres-payment psql -U payment -d payment_db \
  -c "SELECT id, order_id, amount, status FROM payments ORDER BY created_at DESC LIMIT 5;"

# Inventory 테이블
docker exec -it postgres-inventory psql -U inventory -d inventory_db \
  -c "SELECT product_id, product_name, available_quantity, reserved_quantity FROM inventory;"
```

## 🎓 다음 단계

### 멱등성 테스트

```bash
# 같은 idempotencyKey로 재요청
curl -X POST http://localhost:8001/orders \
  -H "Content-Type: application/json" \
  -d '{
    "userId": 1001,
    "amount": 50000,
    "quantity": 1,
    "idempotencyKey": "order-quickstart-001"
  }'
```

**결과:** 같은 주문 ID 반환 (중복 생성 방지)

### Outbox 패턴 확인

```bash
# Outbox 이벤트 확인
docker exec -it postgres-order psql -U order -d order_db \
  -c "SELECT id, event_type, status, created_at, sent_at FROM outbox_events ORDER BY created_at DESC LIMIT 10;"
```

**예상 결과:**
- `PENDING` 상태 → Outbox Worker가 발행 대기
- `SENT` 상태 → Kafka로 발행 완료

## 🧹 정리

```bash
# 모든 서비스 중지
docker compose down

# 볼륨까지 삭제 (DB 데이터 초기화)
docker compose down -v
```

## 🐛 문제 해결

### 서비스가 시작되지 않음

```bash
# 로그 확인
docker compose logs [service-name]

# 개별 서비스 재시작
docker compose restart order-service
```

### Kafka 연결 실패

```bash
# Kafka 재시작
docker compose restart kafka zookeeper

# Kafka 상태 확인
docker exec -it kafka kafka-broker-api-versions.sh --bootstrap-server localhost:9092
```

### DB 연결 실패

```bash
# DB 재시작
docker compose restart postgres-order postgres-payment postgres-inventory postgres-delivery

# DB 상태 확인
docker exec -it postgres-order pg_isready -U order
```

## 📚 추가 학습

- [README.md](README.md) - 전체 프로젝트 문서
- [ARCHITECTURE.md](ARCHITECTURE.md) - 아키텍처 상세 설명
- `make help` - 사용 가능한 명령어 목록

## 💡 유용한 명령어

```bash
# 로그 실시간 모니터링
make logs

# 특정 서비스 로그
make logs-order

# DB 상태 확인
make check-db-order
make check-db-payment
make check-db-inventory

# Redis 확인
make check-redis

# 재고 확인
make check-db-inventory
```

## 🎉 축하합니다!

MSA SAGA 패턴 실습 환경을 성공적으로 구축했습니다. 이제 코드를 탐색하고 커스터마이징해보세요!

### 다음 실습 주제

1. **새로운 서비스 추가**: Notification Service 구현
2. **보상 트랜잭션 강화**: 복잡한 실패 시나리오 처리
3. **모니터링 추가**: Prometheus + Grafana 연동
4. **Temporal Orchestration**: 중앙 집중식 SAGA 구현
5. **부하 테스트**: k6/Gatling으로 성능 측정

---

**질문이나 이슈가 있으시면 GitHub Issues에 등록해주세요!** 🚀

