-- Delivery Service Database Schema

CREATE TYPE delivery_status AS ENUM (
    'PENDING',
    'PREPARING',
    'IN_TRANSIT',
    'DELIVERED',
    'FAILED',
    'CANCELED'
);

-- 배송 테이블
CREATE TABLE IF NOT EXISTS deliveries (
    id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL,
    address VARCHAR(500) NOT NULL,
    recipient_name VARCHAR(100),
    recipient_phone VARCHAR(20),
    status delivery_status NOT NULL DEFAULT 'PENDING',
    idempotency_key VARCHAR(64) NOT NULL UNIQUE,
    tracking_number VARCHAR(100),
    carrier VARCHAR(50),
    reason VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_deliveries_order_id ON deliveries(order_id);
CREATE INDEX idx_deliveries_status ON deliveries(status);
CREATE INDEX idx_deliveries_tracking_number ON deliveries(tracking_number);
CREATE INDEX idx_deliveries_created_at ON deliveries(created_at DESC);

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

-- 배송 히스토리 테이블
CREATE TABLE IF NOT EXISTS delivery_history (
    id BIGSERIAL PRIMARY KEY,
    delivery_id BIGINT NOT NULL,
    status delivery_status NOT NULL,
    location VARCHAR(200),
    description VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_delivery_history_delivery_id ON delivery_history(delivery_id);

COMMENT ON TABLE deliveries IS '배송 테이블';
COMMENT ON TABLE delivery_history IS '배송 상태 변경 히스토리';

