-- Inventory Service Database Schema

CREATE TYPE reservation_status AS ENUM (
    'RESERVED',
    'CONFIRMED',
    'CANCELED',
    'EXPIRED'
);

-- 재고 테이블
CREATE TABLE IF NOT EXISTS inventory (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL UNIQUE,
    product_name VARCHAR(200) NOT NULL,
    available_quantity INT NOT NULL DEFAULT 0,
    reserved_quantity INT NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_inventory_product_id ON inventory(product_id);

-- 재고 예약 테이블
CREATE TABLE IF NOT EXISTS stock_reservations (
    id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    quantity INT NOT NULL,
    status reservation_status NOT NULL DEFAULT 'RESERVED',
    idempotency_key VARCHAR(64) NOT NULL UNIQUE,
    expired_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_stock_reservations_order_id ON stock_reservations(order_id);
CREATE INDEX idx_stock_reservations_product_id ON stock_reservations(product_id);
CREATE INDEX idx_stock_reservations_status ON stock_reservations(status);

-- Outbox 이벤트 테이블
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

-- 샘플 재고 데이터 (테스트용)
INSERT INTO inventory (product_id, product_name, available_quantity) VALUES
(1, 'Sample Product A', 100),
(2, 'Sample Product B', 50),
(3, 'Sample Product C', 200);

COMMENT ON TABLE inventory IS '재고 테이블';
COMMENT ON TABLE stock_reservations IS '재고 예약 테이블';

