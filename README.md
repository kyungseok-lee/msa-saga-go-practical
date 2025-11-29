# MSA 분산 트랜잭션 & SAGA (Golang Practical Guide)

[![Go Version](https://img.shields.io/badge/Go-1.23-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

실무에서 즉시 사용 가능한 Go 기반 MSA 분산 트랜잭션 & SAGA 패턴 구현 예제입니다.

## 📋 목차

- [프로젝트 개요](#프로젝트-개요)
- [아키텍처](#아키텍처)
- [기술 스택](#기술-스택)
- [프로젝트 구조](#프로젝트-구조)
- [시작하기](#시작하기)
- [핵심 패턴](#핵심-패턴)
- [API 사용법](#api-사용법)
- [트러블슈팅](#트러블슈팅)
- [성능 최적화](#성능-최적화)

## 🎯 프로젝트 개요

이 프로젝트는 전자상거래 주문 시나리오를 통해 다음을 실습합니다:

- **Choreography 패턴**: 이벤트 기반 분산 조정
- **Orchestration 패턴**: 중앙 집중식 워크플로우 (Temporal)
- **Outbox 패턴**: 트랜잭션과 메시지 발행의 원자성 보장
- **보상 트랜잭션**: 실패 시 상태 복구
- **멱등성 설계**: 중복 처리 방지
- **Optimistic Locking**: 동시성 제어

### 비즈니스 플로우

```
주문 생성 → 결제 처리 → 재고 예약 → 배송 시작 → 주문 완료
    ↓         ↓          ↓         ↓
   실패      취소       환불      복구
```

## 🏗 아키텍처

### Choreography 패턴 (이벤트 기반)

```
┌──────────┐  OrderCreated   ┌──────────┐  PaymentCompleted  ┌───────────┐
│  Order   │─────────────────→│ Payment  │───────────────────→│ Inventory │
│ Service  │                  │ Service  │                    │  Service  │
└──────────┘                  └──────────┘                    └───────────┘
     ↑                             │                                │
     │                             │ PaymentFailed                  │ StockReserved
     │                             ↓                                ↓
     │                        ┌─────────┐                     ┌──────────┐
     └────────────────────────│  SAGA   │←────────────────────│ Delivery │
                              │  State  │                     │ Service  │
                              └─────────┘                     └──────────┘
```

### 기술 스택

| 카테고리 | 기술 |
|---------|------|
| **언어** | Go 1.23 |
| **데이터베이스** | PostgreSQL 16 |
| **캐시/멱등성** | Redis 7 |
| **메시지 브로커** | Kafka 3.6 (Bitnami) |
| **워크플로우 엔진** | Temporal 1.24 |
| **컨테이너** | Docker, Docker Compose |
| **모니터링** | Kafka UI, Temporal UI |

## 📂 프로젝트 구조

```
msa-saga-go-examples/
├── common/                      # 공통 라이브러리
│   ├── events/                  # 이벤트 정의
│   ├── errors/                  # 에러 코드 및 처리
│   ├── idempotency/            # 멱등성 저장소
│   ├── messaging/              # Kafka 래퍼
│   ├── retry/                  # 재시도 로직
│   └── logger/                 # 로깅 유틸
│
├── services/
│   ├── order/                  # 주문 서비스
│   │   ├── internal/
│   │   │   ├── domain/        # 도메인 모델
│   │   │   ├── repository/    # 데이터 레이어
│   │   │   ├── service/       # 비즈니스 로직
│   │   │   ├── handler/       # HTTP/Event 핸들러
│   │   │   └── worker/        # Outbox Worker
│   │   ├── cmd/main.go
│   │   └── Dockerfile
│   │
│   ├── payment/               # 결제 서비스
│   ├── inventory/             # 재고 서비스
│   └── delivery/              # 배송 서비스
│
├── scripts/                   # DB 초기화 스크립트
│   ├── init-order-db.sql
│   ├── init-payment-db.sql
│   ├── init-inventory-db.sql
│   └── init-delivery-db.sql
│
├── docker compose.yml         # 전체 인프라 정의
├── Makefile                   # 빌드/실행 스크립트
└── README.md
```

## 🚀 시작하기

### 필수 요구사항

- Docker & Docker Compose
- Go 1.23+ (로컬 개발 시)
- Make (선택사항)

### 1. 프로젝트 클론

```bash
git clone <repository-url>
cd msa-saga-go-examples
```

### 2. 인프라 시작

```bash
# 모든 서비스 시작
docker compose up -d

# 로그 확인
docker compose logs -f order-service payment-service inventory-service delivery-service

# 특정 서비스만 재시작
docker compose restart order-service
```

### 3. 서비스 상태 확인

```bash
# Order Service
curl http://localhost:8001/health

# Payment Service
curl http://localhost:8002/health

# Inventory Service
curl http://localhost:8003/health

# Delivery Service
curl http://localhost:8004/health
```

### 4. UI 접속

- **Kafka UI**: http://localhost:8080
- **Temporal UI**: http://localhost:8088

## 🔑 핵심 패턴

### 1. Outbox 패턴

트랜잭션과 메시지 발행의 원자성을 보장합니다.

```go
// Order Service - CreateOrder
func (s *orderService) CreateOrder(ctx context.Context, cmd CreateOrderCommand) (*CreateOrderResult, error) {
    tx, _ := s.db.BeginTx(ctx, nil)
    defer tx.Rollback()

    // 1. 주문 생성 (DB 트랜잭션)
    order := &domain.Order{...}
    s.orderRepo.Create(ctx, order)

    // 2. Outbox 이벤트 저장 (같은 트랜잭션)
    outboxEvent := &repository.OutboxEvent{
        EventType: "order.created.v1",
        Payload:   marshal(OrderCreatedEvent{...}),
        Status:    "PENDING",
    }
    s.outboxRepo.InsertTx(ctx, tx, outboxEvent)

    // 3. 트랜잭션 커밋 (원자성 보장)
    tx.Commit()

    return result, nil
}
```

**Outbox Worker**가 주기적으로 `PENDING` 이벤트를 Kafka로 발행합니다.

### 2. 멱등성 (Idempotency) 설계

중복 요청/메시지 처리를 방지합니다.

```go
// Payment Service - HandleOrderCreated
func (s *paymentService) HandleOrderCreated(ctx context.Context, evt events.OrderCreatedEvent) error {
    // 멱등성 키 생성
    idempotencyKey := fmt.Sprintf("payment-%d-%s", evt.OrderID, evt.EventID)

    // 이미 처리된 요청 확인
    existing, err := s.paymentRepo.FindByIdempotencyKey(ctx, idempotencyKey)
    if err == nil {
        return nil // 이미 처리됨
    }

    // 결제 처리...
}
```

Redis를 사용한 멱등성 체크:

```go
// Event Handler
if processed, _ := h.idemStore.IsProcessed(ctx, evt.EventID); processed {
    return nil
}

// 처리 후 기록
_ = h.idemStore.Reserve(ctx, evt.EventID, 24*time.Hour)
```

### 3. 보상 트랜잭션 (Compensation)

재고 예약 실패 시 결제 환불을 수행합니다.

```go
// Payment Service - 보상 트랜잭션
func (s *paymentService) HandleStockReservationFailed(
    ctx context.Context,
    evt events.StockReservationFailedEvent,
) error {
    // 결제 조회
    payment, _ := s.paymentRepo.FindByOrderID(ctx, evt.OrderID)

    // 결제 환불 (외부 게이트웨이 호출)
    s.refundPayment(ctx, payment)

    // 상태 업데이트 & 환불 이벤트 발행
    s.paymentRepo.UpdateStatus(ctx, payment.ID, domain.PaymentStatusRefunded, evt.Reason)
    
    // PaymentRefunded 이벤트 발행 → Inventory가 재고 복구
}
```

### 4. Semantic Lock (상태 기반 잠금)

```go
// Order Domain Model
func (o *Order) CanTransitionTo(newStatus OrderStatus) bool {
    transitions := map[OrderStatus][]OrderStatus{
        OrderStatusPending: {
            OrderStatusPaymentProcessing,
            OrderStatusCanceled,
        },
        // ...
    }
    // 허용된 전이만 가능
}
```

### 5. Optimistic Locking (낙관적 잠금)

```go
// Inventory Service - 재고 차감
result, err := tx.ExecContext(ctx, `
    UPDATE inventory
    SET available_quantity = available_quantity - $1,
        version = version + 1
    WHERE product_id = $2 AND version = $3
`, quantity, productID, currentVersion)

if affected == 0 {
    return errors.New("version conflict")
}
```

## 📡 API 사용법

### 주문 생성 (성공 시나리오)

```bash
curl -X POST http://localhost:8001/orders \
  -H "Content-Type: application/json" \
  -d '{
    "userId": 1001,
    "amount": 50000,
    "quantity": 1,
    "idempotencyKey": "order-20250129-001"
  }'
```

**응답:**
```json
{
  "orderId": 123,
  "status": "PENDING"
}
```

**SAGA 흐름:**
1. Order Service: 주문 생성 → `OrderCreated` 이벤트 발행
2. Payment Service: 결제 처리 → `PaymentCompleted` 이벤트 발행
3. Inventory Service: 재고 예약 → `StockReserved` 이벤트 발행
4. Delivery Service: 배송 시작 → `DeliveryStarted` 이벤트 발행
5. Order Service: 주문 상태 → `COMPLETED`

### 주문 조회

```bash
curl http://localhost:8001/orders/123
```

**응답:**
```json
{
  "id": 123,
  "userId": 1001,
  "amount": 50000,
  "quantity": 1,
  "status": "COMPLETED",
  "createdAt": "2025-01-29T10:00:00Z",
  "updatedAt": "2025-01-29T10:00:15Z"
}
```

### 실패 시나리오 (재고 부족)

**SAGA 흐름:**
1. Order Service: 주문 생성 → `OrderCreated`
2. Payment Service: 결제 완료 → `PaymentCompleted`
3. Inventory Service: 재고 부족 → `StockReservationFailed` 이벤트 발행
4. **Payment Service: 보상 트랜잭션 - 결제 환불** → `PaymentRefunded`
5. **Inventory Service: 재고 복구** (필요 시)
6. Order Service: 주문 상태 → `CANCELED` or `FAILED`

## 🐛 트러블슈팅

### 1. Kafka 연결 실패

```bash
# Kafka 상태 확인
docker compose logs kafka

# Kafka 재시작
docker compose restart kafka zookeeper
```

### 2. DB 마이그레이션 실패

```bash
# DB 재초기화
docker compose down -v
docker compose up -d postgres-order postgres-payment postgres-inventory postgres-delivery
```

### 3. Outbox 이벤트가 발행되지 않음

```bash
# Outbox Worker 로그 확인
docker compose logs -f order-service | grep "outbox"

# Outbox 테이블 확인
docker exec -it postgres-order psql -U order -d order_db \
  -c "SELECT * FROM outbox_events WHERE status = 'PENDING';"
```

### 4. 멱등성 체크 실패

```bash
# Redis 연결 확인
docker compose logs redis

# Redis CLI 접속
docker exec -it redis redis-cli
> KEYS *
> GET "order-service:event-id-xxxx"
```

## 📊 성능 최적화

### Kafka 파티셔닝 전략

- **파티션 키**: `orderId`를 사용하여 주문 단위 순서 보장
- **파티션 수**: 서비스 인스턴스 수 ≥ 파티션 수

### Database Connection Pool

```go
db.SetMaxOpenConns(25)      // 최대 연결 수
db.SetMaxIdleConns(10)      // 유휴 연결 수
db.SetConnMaxLifetime(5 * time.Minute)
```

### Redis TTL 설정

```go
// 멱등성 키 TTL: 24시간
idemStore.Reserve(ctx, eventID, 24*time.Hour)
```

## 🔍 모니터링

### Kafka UI

http://localhost:8080

- 토픽별 메시지 확인
- Consumer Group 상태
- Lag 모니터링

### Temporal UI (Orchestration 패턴)

http://localhost:8088

- Workflow 실행 히스토리
- 실패한 Activity 재시도
- 수동 개입

### 로그 조회

```bash
# 전체 서비스 로그
docker compose logs -f

# 특정 서비스 로그
docker compose logs -f order-service

# 에러 로그만 필터
docker compose logs order-service | grep ERROR
```

### 데이터베이스 및 볼륨 확인

```bash
# Docker 볼륨 정보 확인
make check-volumes

# Order DB 데이터 확인
make check-db-order

# Payment DB 데이터 확인
make check-db-payment

# Inventory DB 데이터 확인
make check-db-inventory

# Redis 키 확인
make check-redis
```

## 🧪 테스트

### 단위 테스트

```bash
# 전체 테스트 실행
go test ./...

# 특정 패키지 테스트
go test ./services/order/internal/service/...

# 커버리지 확인
go test -cover ./...
```

### 통합 테스트

```bash
# 환경 시작
docker compose up -d

# E2E 테스트 실행
go test ./tests/e2e/... -v
```

## 📈 확장 포인트

1. **Temporal Orchestration 패턴 구현** (TODO: 추가 예정)
2. **분산 트레이싱** (OpenTelemetry + Jaeger)
3. **메트릭 수집** (Prometheus + Grafana)
4. **API Gateway** (Kong, Envoy)
5. **Service Mesh** (Istio)

## 🤝 기여 가이드

1. Fork the Project
2. Create your Feature Branch (`git checkout -b feature/AmazingFeature`)
3. Commit your Changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the Branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## 📝 라이센스

MIT License

## 📚 참고 자료

- [Sagas (Garcia-Molina, Salem)](https://www.cs.cornell.edu/andru/cs711/2002fa/reading/sagas.pdf)
- [Temporal Documentation](https://docs.temporal.io/)
- [Kafka Documentation](https://kafka.apache.org/documentation/)
- [Outbox Pattern](https://microservices.io/patterns/data/transactional-outbox.html)

## 👥 저자

Backend Engineer specializing in MSA & Distributed Systems

---

**⭐ 이 프로젝트가 도움이 되었다면 Star를 눌러주세요!**

