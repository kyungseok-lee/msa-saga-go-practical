package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kyungseok/msa-saga-go-practical/common/events"
	"github.com/kyungseok/msa-saga-go-practical/common/idempotency"
	"github.com/kyungseok/msa-saga-go-practical/common/messaging"
	"github.com/kyungseok/msa-saga-go-practical/services/payment/internal/service"
	"go.uber.org/zap"
)

// EventHandler 이벤트 핸들러
type EventHandler struct {
	paymentService service.PaymentService
	idemStore      idempotency.Store
	logger         *zap.Logger
}

// NewEventHandler 이벤트 핸들러 생성
func NewEventHandler(
	paymentService service.PaymentService,
	idemStore idempotency.Store,
	logger *zap.Logger,
) *EventHandler {
	return &EventHandler{
		paymentService: paymentService,
		idemStore:      idemStore,
		logger:         logger,
	}
}

// HandleMessage 메시지 처리
func (h *EventHandler) HandleMessage(ctx context.Context, msg *messaging.Message) error {
	h.logger.Info("received message",
		zap.String("topic", msg.Topic),
		zap.Int64("offset", msg.Offset))

	// 이벤트 타입에 따라 분기
	switch events.EventType(msg.Topic) {
	case events.EventOrderCreated:
		return h.handleOrderCreated(ctx, msg)
	case events.EventStockReservationFailed:
		return h.handleStockReservationFailed(ctx, msg)
	default:
		h.logger.Warn("unknown event type", zap.String("topic", msg.Topic))
		return nil
	}
}

func (h *EventHandler) handleOrderCreated(ctx context.Context, msg *messaging.Message) error {
	var evt events.OrderCreatedEvent
	if err := json.Unmarshal(msg.Value, &evt); err != nil {
		return err
	}

	// 멱등성 체크
	if processed, _ := h.idemStore.IsProcessed(ctx, evt.EventID); processed {
		h.logger.Info("event already processed", zap.String("eventId", evt.EventID))
		return nil
	}

	if err := h.paymentService.HandleOrderCreated(ctx, evt); err != nil {
		return err
	}

	// 처리 완료 표시
	_ = h.idemStore.Reserve(ctx, evt.EventID, 24*time.Hour)
	return nil
}

func (h *EventHandler) handleStockReservationFailed(ctx context.Context, msg *messaging.Message) error {
	var evt events.StockReservationFailedEvent
	if err := json.Unmarshal(msg.Value, &evt); err != nil {
		return err
	}

	// 멱등성 체크
	if processed, _ := h.idemStore.IsProcessed(ctx, evt.EventID); processed {
		h.logger.Info("event already processed", zap.String("eventId", evt.EventID))
		return nil
	}

	if err := h.paymentService.HandleStockReservationFailed(ctx, evt); err != nil {
		return err
	}

	// 처리 완료 표시
	_ = h.idemStore.Reserve(ctx, evt.EventID, 24*time.Hour)
	return nil
}

