package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/kyungseok/msa-saga-go-practical/common/errors"
	"github.com/kyungseok/msa-saga-go-practical/common/events"
	"github.com/kyungseok/msa-saga-go-practical/services/payment/internal/domain"
	"github.com/kyungseok/msa-saga-go-practical/services/payment/internal/repository"
	"go.uber.org/zap"
)

// PaymentService 결제 서비스 인터페이스
type PaymentService interface {
	HandleOrderCreated(ctx context.Context, evt events.OrderCreatedEvent) error
	HandleStockReservationFailed(ctx context.Context, evt events.StockReservationFailedEvent) error
	GetPayment(ctx context.Context, paymentID int64) (*domain.Payment, error)
}

type paymentService struct {
	db          *sql.DB
	paymentRepo repository.PaymentRepository
	outboxRepo  repository.OutboxRepository
	logger      *zap.Logger
}

// NewPaymentService 결제 서비스 생성
func NewPaymentService(
	db *sql.DB,
	paymentRepo repository.PaymentRepository,
	outboxRepo repository.OutboxRepository,
	logger *zap.Logger,
) PaymentService {
	return &paymentService{
		db:          db,
		paymentRepo: paymentRepo,
		outboxRepo:  outboxRepo,
		logger:      logger,
	}
}

// HandleOrderCreated 주문 생성 이벤트 처리 (결제 실행)
func (s *paymentService) HandleOrderCreated(ctx context.Context, evt events.OrderCreatedEvent) error {
	s.logger.Info("handling order created event",
		zap.Int64("orderId", evt.OrderID),
		zap.String("correlationId", evt.CorrelationID))

	// 멱등성 키 생성
	idempotencyKey := fmt.Sprintf("payment-%d-%s", evt.OrderID, evt.EventID)

	// 이미 처리된 요청인지 확인
	existing, err := s.paymentRepo.FindByIdempotencyKey(ctx, idempotencyKey)
	if err == nil {
		s.logger.Info("payment already processed",
			zap.String("idempotencyKey", idempotencyKey),
			zap.Int64("paymentId", existing.ID))
		return nil
	}

	// 트랜잭션 시작
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to begin transaction", err)
	}
	defer tx.Rollback()

	// 결제 처리 (외부 결제 게이트웨이 호출 시뮬레이션)
	paymentResult, err := s.processPayment(ctx, evt.OrderID, evt.Amount)
	if err != nil {
		// 결제 실패 이벤트 발행
		return s.publishPaymentFailed(ctx, tx, evt, err.Error())
	}

	// 결제 기록 생성
	now := time.Now()
	payment := &domain.Payment{
		OrderID:            evt.OrderID,
		Amount:             evt.Amount,
		PaymentType:        "CARD",
		Status:             domain.PaymentStatusCompleted,
		IdempotencyKey:     idempotencyKey,
		PaymentGatewayTxID: paymentResult.TransactionID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := s.paymentRepo.Create(ctx, payment); err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to create payment", err)
	}

	// 결제 완료 이벤트 발행
	paymentCompletedEvt := events.PaymentCompletedEvent{
		BaseEvent: events.BaseEvent{
			EventID:       uuid.New().String(),
			EventType:     events.EventPaymentCompleted,
			SchemaVersion: 1,
			OccurredAt:    now,
			CorrelationID: evt.CorrelationID,
		},
		OrderID:     evt.OrderID,
		PaymentID:   payment.ID,
		Amount:      payment.Amount,
		PaymentType: payment.PaymentType,
	}

	payload, err := json.Marshal(paymentCompletedEvt)
	if err != nil {
		return errors.Wrap(errors.ErrCodeSerializationError, "failed to marshal event", err)
	}

	outboxEvent := &repository.OutboxEvent{
		AggregateType: "payment",
		AggregateID:   payment.ID,
		EventType:     string(events.EventPaymentCompleted),
		Payload:       payload,
		Status:        "PENDING",
		CreatedAt:     now,
	}

	if err := s.outboxRepo.InsertTx(ctx, tx, outboxEvent); err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to insert outbox event", err)
	}

	// 트랜잭션 커밋
	if err := tx.Commit(); err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to commit transaction", err)
	}

	s.logger.Info("payment completed",
		zap.Int64("paymentId", payment.ID),
		zap.Int64("orderId", evt.OrderID),
		zap.Int64("amount", payment.Amount))

	return nil
}

// HandleStockReservationFailed 재고 예약 실패 이벤트 처리 (보상 트랜잭션: 결제 환불)
func (s *paymentService) HandleStockReservationFailed(ctx context.Context, evt events.StockReservationFailedEvent) error {
	s.logger.Warn("handling stock reservation failed event - initiating refund",
		zap.Int64("orderId", evt.OrderID),
		zap.String("reason", evt.Reason))

	// 해당 주문의 결제 조회
	payment, err := s.paymentRepo.FindByOrderID(ctx, evt.OrderID)
	if err != nil {
		s.logger.Error("payment not found for refund",
			zap.Int64("orderId", evt.OrderID),
			zap.Error(err))
		return err
	}

	// 이미 환불된 경우 스킵
	if payment.Status == domain.PaymentStatusRefunded {
		s.logger.Info("payment already refunded", zap.Int64("paymentId", payment.ID))
		return nil
	}

	// 트랜잭션 시작
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to begin transaction", err)
	}
	defer tx.Rollback()

	// 결제 환불 처리 (외부 결제 게이트웨이 호출 시뮬레이션)
	if err := s.refundPayment(ctx, payment); err != nil {
		s.logger.Error("failed to refund payment",
			zap.Int64("paymentId", payment.ID),
			zap.Error(err))
		return err
	}

	// 결제 상태 업데이트
	if err := s.paymentRepo.UpdateStatus(ctx, payment.ID, domain.PaymentStatusRefunded, evt.Reason); err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to update payment status", err)
	}

	// 결제 환불 이벤트 발행
	now := time.Now()
	paymentRefundedEvt := events.PaymentRefundedEvent{
		BaseEvent: events.BaseEvent{
			EventID:       uuid.New().String(),
			EventType:     events.EventPaymentRefunded,
			SchemaVersion: 1,
			OccurredAt:    now,
			CorrelationID: evt.CorrelationID,
		},
		OrderID:   evt.OrderID,
		PaymentID: payment.ID,
		Amount:    payment.Amount,
	}

	payload, err := json.Marshal(paymentRefundedEvt)
	if err != nil {
		return errors.Wrap(errors.ErrCodeSerializationError, "failed to marshal event", err)
	}

	outboxEvent := &repository.OutboxEvent{
		AggregateType: "payment",
		AggregateID:   payment.ID,
		EventType:     string(events.EventPaymentRefunded),
		Payload:       payload,
		Status:        "PENDING",
		CreatedAt:     now,
	}

	if err := s.outboxRepo.InsertTx(ctx, tx, outboxEvent); err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to insert outbox event", err)
	}

	// 트랜잭션 커밋
	if err := tx.Commit(); err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to commit transaction", err)
	}

	s.logger.Info("payment refunded successfully",
		zap.Int64("paymentId", payment.ID),
		zap.Int64("orderId", evt.OrderID))

	return nil
}

// GetPayment 결제 조회
func (s *paymentService) GetPayment(ctx context.Context, paymentID int64) (*domain.Payment, error) {
	return s.paymentRepo.FindByID(ctx, paymentID)
}

// processPayment 결제 처리 (외부 결제 게이트웨이 호출 시뮬레이션)
func (s *paymentService) processPayment(ctx context.Context, orderID, amount int64) (*PaymentResult, error) {
	// 실제로는 외부 결제 게이트웨이 API를 호출
	// 여기서는 시뮬레이션: 10% 확률로 결제 실패
	time.Sleep(100 * time.Millisecond) // 네트워크 지연 시뮬레이션

	if rand.Intn(100) < 10 {
		return nil, errors.New(errors.ErrCodePaymentDeclined, "payment declined by gateway")
	}

	return &PaymentResult{
		TransactionID: fmt.Sprintf("PG-TXN-%d-%d", orderID, time.Now().Unix()),
		Status:        "SUCCESS",
	}, nil
}

// refundPayment 결제 환불 처리 (외부 결제 게이트웨이 호출 시뮬레이션)
func (s *paymentService) refundPayment(ctx context.Context, payment *domain.Payment) error {
	// 실제로는 외부 결제 게이트웨이 환불 API를 호출
	time.Sleep(100 * time.Millisecond) // 네트워크 지연 시뮬레이션

	s.logger.Info("refund processed successfully",
		zap.String("paymentGatewayTxId", payment.PaymentGatewayTxID))

	return nil
}

func (s *paymentService) publishPaymentFailed(
	ctx context.Context,
	tx *sql.Tx,
	evt events.OrderCreatedEvent,
	reason string,
) error {
	now := time.Now()
	paymentFailedEvt := events.PaymentFailedEvent{
		BaseEvent: events.BaseEvent{
			EventID:       uuid.New().String(),
			EventType:     events.EventPaymentFailed,
			SchemaVersion: 1,
			OccurredAt:    now,
			CorrelationID: evt.CorrelationID,
		},
		OrderID: evt.OrderID,
		Reason:  reason,
	}

	payload, err := json.Marshal(paymentFailedEvt)
	if err != nil {
		return errors.Wrap(errors.ErrCodeSerializationError, "failed to marshal event", err)
	}

	outboxEvent := &repository.OutboxEvent{
		AggregateType: "payment",
		AggregateID:   evt.OrderID,
		EventType:     string(events.EventPaymentFailed),
		Payload:       payload,
		Status:        "PENDING",
		CreatedAt:     now,
	}

	if err := s.outboxRepo.InsertTx(ctx, tx, outboxEvent); err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to insert outbox event", err)
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(errors.ErrCodeDatabaseError, "failed to commit transaction", err)
	}

	s.logger.Warn("payment failed event published",
		zap.Int64("orderId", evt.OrderID),
		zap.String("reason", reason))

	return fmt.Errorf("payment failed: %s", reason)
}

// PaymentResult 결제 결과
type PaymentResult struct {
	TransactionID string
	Status        string
}

