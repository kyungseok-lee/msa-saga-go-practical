-- Order Service Database Schema

-- 주문 상태 정의
-- PENDING: 주문 생성됨
-- PAYMENT_PROCESSING: 결제 처리 중
-- STOCK_RESERVING: 재고 예약 중
-- DELIVERY_PREPARING: 배송 준비 중
-- COMPLETED: 완료
-- CANCELED: 취소됨
-- FAILED: 실패

CREATE TYPE order_status AS ENUM (
    'PENDING',
    'PAYMENT_PROCESSING',
    'STOCK_RESERVING',
    'DELIVERY_PREPARING',
    'COMPLETED',
    'CANCELED',
    'FAILED'
);

-- 주문 테이블
CREATE TABLE IF NOT EXISTS orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    amount BIGINT NOT NULL,
    quantity INT NOT NULL,
    status order_status NOT NULL DEFAULT 'PENDING',
    version BIGINT NOT NULL DEFAULT 0,
    idempotency_key VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_orders_idempotency_key ON orders(idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_orders_user_id ON orders(user_id);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_created_at ON orders(created_at DESC);

-- Outbox 이벤트 테이블 (트랜잭션과 메시지 발행의 원자성 보장)
CREATE TABLE IF NOT EXISTS outbox_events (
    id BIGSERIAL PRIMARY KEY,
    aggregate_type VARCHAR(50) NOT NULL,
    aggregate_id BIGINT NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ
);

CREATE INDEX idx_outbox_events_status_created_at ON outbox_events(status, created_at);
CREATE INDEX idx_outbox_events_aggregate ON outbox_events(aggregate_type, aggregate_id);

-- SAGA 상태 추적 테이블
CREATE TABLE IF NOT EXISTS saga_instances (
    id BIGSERIAL PRIMARY KEY,
    saga_id VARCHAR(64) NOT NULL UNIQUE,
    saga_type VARCHAR(50) NOT NULL,
    order_id BIGINT NOT NULL,
    status VARCHAR(20) NOT NULL,
    current_step VARCHAR(50),
    payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_saga_instances_order_id ON saga_instances(order_id);
CREATE INDEX idx_saga_instances_status ON saga_instances(status);

-- 샘플 데이터 (테스트용)
-- 실제 운영에서는 제거하거나 주석 처리

COMMENT ON TABLE orders IS '주문 테이블';
COMMENT ON TABLE outbox_events IS 'Outbox 패턴을 위한 이벤트 테이블';
COMMENT ON TABLE saga_instances IS 'SAGA 인스턴스 추적 테이블';

