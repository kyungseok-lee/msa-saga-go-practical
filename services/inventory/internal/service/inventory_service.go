package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kyungseok/msa-saga-go-examples/common/errors"
	"github.com/kyungseok/msa-saga-go-examples/common/events"
	"go.uber.org/zap"
)

// InventoryService 재고 서비스 인터페이스
type InventoryService interface {
	HandlePaymentCompleted(ctx context.Context, evt events.PaymentCompletedEvent) error
	HandlePaymentRefunded(ctx context.Context, evt events.PaymentRefundedEvent) error
}

type inventoryService struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewInventoryService 재고 서비스 생성
func NewInventoryService(db *sql.DB, logger *zap.Logger) InventoryService {
	return &inventoryService{
		db:     db,
		logger: logger,
	}
}

// HandlePaymentCompleted 결제 완료 이벤트 처리 (재고 예약)
func (s *inventoryService) HandlePaymentCompleted(ctx context.Context, evt events.PaymentCompletedEvent) error {
	s.logger.Info("handling payment completed event - reserving stock",
		zap.Int64("orderId", evt.OrderID))

	// 멱등성 키 생성
	idempotencyKey := fmt.Sprintf("stock-reservation-%d-%s", evt.OrderID, evt.EventID)

	// 이미 처리된 요청인지 확인
	var existingReservationID int64
	err := s.db.QueryRowContext(ctx, `
		SELECT id FROM stock_reservations WHERE idempotency_key = $1
	`, idempotencyKey).Scan(&existingReservationID)

	if err == nil {
		s.logger.Info("stock already reserved", zap.String("idempotencyKey", idempotencyKey))
		return nil
	}

	// 트랜잭션 시작
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to begin transaction", err)
	}
	defer tx.Rollback()

	// 재고 확인 및 예약 (단순화: productID=1로 가정)
	productID := int64(1)
	quantity := 1 // evt.Quantity 사용 가능

	var availableQty int
	var version int64
	err = tx.QueryRowContext(ctx, `
		SELECT available_quantity, version FROM inventory WHERE product_id = $1 FOR UPDATE
	`, productID).Scan(&availableQty, &version)

	if err != nil {
		return s.publishStockReservationFailed(ctx, tx, evt, "product not found")
	}

	if availableQty < quantity {
		return s.publishStockReservationFailed(ctx, tx, evt, "insufficient stock")
	}

	// 재고 차감 (Optimistic Lock)
	result, err := tx.ExecContext(ctx, `
		UPDATE inventory
		SET available_quantity = available_quantity - $1,
			reserved_quantity = reserved_quantity + $1,
			version = version + 1,
			updated_at = NOW()
		WHERE product_id = $2 AND version = $3
	`, quantity, productID, version)

	if err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to update inventory", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return s.publishStockReservationFailed(ctx, tx, evt, "version conflict")
	}

	// 재고 예약 기록 생성
	var reservationID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO stock_reservations (order_id, product_id, quantity, status, idempotency_key, expired_at, created_at, updated_at)
		VALUES ($1, $2, $3, 'RESERVED', $4, NOW() + INTERVAL '30 minutes', NOW(), NOW())
		RETURNING id
	`, evt.OrderID, productID, quantity, idempotencyKey).Scan(&reservationID)

	if err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to create reservation", err)
	}

	// StockReserved 이벤트 발행
	now := time.Now()
	stockReservedEvt := events.StockReservedEvent{
		BaseEvent: events.BaseEvent{
			EventID:       uuid.New().String(),
			EventType:     events.EventStockReserved,
			SchemaVersion: 1,
			OccurredAt:    now,
			CorrelationID: evt.CorrelationID,
		},
		OrderID:       evt.OrderID,
		ReservationID: reservationID,
		Quantity:      quantity,
	}

	payload, _ := json.Marshal(stockReservedEvt)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, status, created_at)
		VALUES ('stock_reservation', $1, $2, $3, 'PENDING', NOW())
	`, reservationID, string(events.EventStockReserved), payload)

	if err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to insert outbox event", err)
	}

	// 트랜잭션 커밋
	if err := tx.Commit(); err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to commit transaction", err)
	}

	s.logger.Info("stock reserved successfully",
		zap.Int64("reservationId", reservationID),
		zap.Int64("orderId", evt.OrderID))

	return nil
}

// HandlePaymentRefunded 결제 환불 이벤트 처리 (재고 복구 - 보상 트랜잭션)
func (s *inventoryService) HandlePaymentRefunded(ctx context.Context, evt events.PaymentRefundedEvent) error {
	s.logger.Warn("handling payment refunded event - restoring stock",
		zap.Int64("orderId", evt.OrderID))

	// 해당 주문의 재고 예약 조회
	var reservationID int64
	var productID int64
	var quantity int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, product_id, quantity FROM stock_reservations
		WHERE order_id = $1 AND status = 'RESERVED'
	`, evt.OrderID).Scan(&reservationID, &productID, &quantity)

	if err != nil {
		s.logger.Error("stock reservation not found", zap.Int64("orderId", evt.OrderID))
		return nil // 이미 복구되었거나 예약이 없음
	}

	// 트랜잭션 시작
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 재고 복구
	_, err = tx.ExecContext(ctx, `
		UPDATE inventory
		SET available_quantity = available_quantity + $1,
			reserved_quantity = reserved_quantity - $1,
			version = version + 1,
			updated_at = NOW()
		WHERE product_id = $2
	`, quantity, productID)

	if err != nil {
		return err
	}

	// 예약 상태 업데이트
	_, err = tx.ExecContext(ctx, `
		UPDATE stock_reservations
		SET status = 'CANCELED', updated_at = NOW()
		WHERE id = $1
	`, reservationID)

	if err != nil {
		return err
	}

	// StockRestored 이벤트 발행
	now := time.Now()
	stockRestoredEvt := events.StockRestoredEvent{
		BaseEvent: events.BaseEvent{
			EventID:       uuid.New().String(),
			EventType:     events.EventStockRestored,
			SchemaVersion: 1,
			OccurredAt:    now,
			CorrelationID: evt.CorrelationID,
		},
		OrderID:       evt.OrderID,
		ReservationID: reservationID,
		Quantity:      quantity,
	}

	payload, _ := json.Marshal(stockRestoredEvt)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, status, created_at)
		VALUES ('stock_reservation', $1, $2, $3, 'PENDING', NOW())
	`, reservationID, string(events.EventStockRestored), payload)

	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	s.logger.Info("stock restored successfully",
		zap.Int64("reservationId", reservationID),
		zap.Int64("orderId", evt.OrderID))

	return nil
}

func (s *inventoryService) publishStockReservationFailed(
	ctx context.Context,
	tx *sql.Tx,
	evt events.PaymentCompletedEvent,
	reason string,
) error {
	now := time.Now()
	failedEvt := events.StockReservationFailedEvent{
		BaseEvent: events.BaseEvent{
			EventID:       uuid.New().String(),
			EventType:     events.EventStockReservationFailed,
			SchemaVersion: 1,
			OccurredAt:    now,
			CorrelationID: evt.CorrelationID,
		},
		OrderID:  evt.OrderID,
		Quantity: 1,
		Reason:   reason,
	}

	payload, _ := json.Marshal(failedEvt)

	_, err := tx.ExecContext(ctx, `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, status, created_at)
		VALUES ('order', $1, $2, $3, 'PENDING', NOW())
	`, evt.OrderID, string(events.EventStockReservationFailed), payload)

	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	s.logger.Warn("stock reservation failed event published",
		zap.Int64("orderId", evt.OrderID),
		zap.String("reason", reason))

	return fmt.Errorf("stock reservation failed: %s", reason)
}
