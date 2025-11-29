package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kyungseok/msa-saga-go-practical/common/errors"
	"github.com/kyungseok/msa-saga-go-practical/common/events"
	"go.uber.org/zap"
)

// DeliveryService 배송 서비스 인터페이스
type DeliveryService interface {
	HandleStockReserved(ctx context.Context, evt events.StockReservedEvent) error
}

type deliveryService struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewDeliveryService 배송 서비스 생성
func NewDeliveryService(db *sql.DB, logger *zap.Logger) DeliveryService {
	return &deliveryService{
		db:     db,
		logger: logger,
	}
}

// HandleStockReserved 재고 예약 이벤트 처리 (배송 시작)
func (s *deliveryService) HandleStockReserved(ctx context.Context, evt events.StockReservedEvent) error {
	s.logger.Info("handling stock reserved event - starting delivery",
		zap.Int64("orderId", evt.OrderID))

	// 멱등성 키 생성
	idempotencyKey := fmt.Sprintf("delivery-%d-%s", evt.OrderID, evt.EventID)

	// 이미 처리된 요청인지 확인
	var existingDeliveryID int64
	err := s.db.QueryRowContext(ctx, `
		SELECT id FROM deliveries WHERE idempotency_key = $1
	`, idempotencyKey).Scan(&existingDeliveryID)

	if err == nil {
		s.logger.Info("delivery already started", zap.String("idempotencyKey", idempotencyKey))
		return nil
	}

	// 트랜잭션 시작
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to begin transaction", err)
	}
	defer tx.Rollback()

	// 배송 정보 생성
	var deliveryID int64
	trackingNumber := fmt.Sprintf("TRK-%d-%d", evt.OrderID, time.Now().Unix())
	address := "서울시 강남구 테헤란로 123" // 실제로는 주문 정보에서 가져와야 함

	err = tx.QueryRowContext(ctx, `
		INSERT INTO deliveries (order_id, address, status, idempotency_key, tracking_number, carrier, created_at, updated_at)
		VALUES ($1, $2, 'PREPARING', $3, $4, 'CJ대한통운', NOW(), NOW())
		RETURNING id
	`, evt.OrderID, address, idempotencyKey, trackingNumber).Scan(&deliveryID)

	if err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to create delivery", err)
	}

	// DeliveryStarted 이벤트 발행
	now := time.Now()
	deliveryStartedEvt := events.DeliveryStartedEvent{
		BaseEvent: events.BaseEvent{
			EventID:       uuid.New().String(),
			EventType:     events.EventDeliveryStarted,
			SchemaVersion: 1,
			OccurredAt:    now,
			CorrelationID: evt.CorrelationID,
		},
		OrderID:    evt.OrderID,
		DeliveryID: deliveryID,
		Address:    address,
	}

	payload, _ := json.Marshal(deliveryStartedEvt)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, status, created_at)
		VALUES ('delivery', $1, $2, $3, 'PENDING', NOW())
	`, deliveryID, string(events.EventDeliveryStarted), payload)

	if err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to insert outbox event", err)
	}

	// 트랜잭션 커밋
	if err := tx.Commit(); err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to commit transaction", err)
	}

	s.logger.Info("delivery started successfully",
		zap.Int64("deliveryId", deliveryID),
		zap.Int64("orderId", evt.OrderID),
		zap.String("trackingNumber", trackingNumber))

	return nil
}

