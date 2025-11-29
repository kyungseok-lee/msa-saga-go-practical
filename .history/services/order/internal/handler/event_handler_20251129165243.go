package handler

import (
	"context"
	"encoding/json"

	"github.com/kyungseok/msa-saga-go-practical/common/events"
	"github.com/kyungseok/msa-saga-go-practical/common/idempotency"
	"github.com/kyungseok/msa-saga-go-practical/common/messaging"
	"github.com/kyungseok/msa-saga-go-practical/services/order/internal/service"
	"go.uber.org/zap"
	"time"
)

// EventHandler 이벤트 핸들러
type EventHandler struct {
	orderService service.OrderService
	idemStore    idempotency.Store
	logger       *zap.Logger
}

// NewEventHandler 이벤트 핸들러 생성
func NewEventHandler(
	orderService service.OrderService,
	idemStore idempotency.Store,
	logger *zap.Logger,
) *EventHandler {
	return &EventHandler{
		orderService: orderService,
		idemStore:    idemStore,
		logger:       logger,
	}
}

// HandleMessage 메시지 처리
func (h *EventHandler) HandleMessage(ctx context.Context, msg *messaging.Message) error {
	h.logger.Info("received message",
		zap.String("topic", msg.Topic),
		zap.Int64("offset", msg.Offset))

	// 이벤트 타입에 따라 분기
	switch events.EventType(msg.Topic) {
	case events.EventPaymentCompleted:
		return h.handlePaymentCompleted(ctx, msg)
	case events.EventPaymentFailed:
		return h.handlePaymentFailed(ctx, msg)
	case events.EventStockReserved:
		return h.handleStockReserved(ctx, msg)
	case events.EventStockReservationFailed:
		return h.handleStockReservationFailed(ctx, msg)
	case events.EventDeliveryStarted:
		return h.handleDeliveryStarted(ctx, msg)
	case events.EventDeliveryFailed:
		return h.handleDeliveryFailed(ctx, msg)
	default:
		h.logger.Warn("unknown event type", zap.String("topic", msg.Topic))
		return nil
	}
}

func (h *EventHandler) handlePaymentCompleted(ctx context.Context, msg *messaging.Message) error {
	var evt events.PaymentCompletedEvent
	if err := json.Unmarshal(msg.Value, &evt); err != nil {
		return err
	}

	// 멱등성 체크
	if processed, _ := h.idemStore.IsProcessed(ctx, evt.EventID); processed {
		h.logger.Info("event already processed", zap.String("eventId", evt.EventID))
		return nil
	}

	if err := h.orderService.HandlePaymentCompleted(ctx, evt); err != nil {
		return err
	}

	// 처리 완료 표시
	_ = h.idemStore.Reserve(ctx, evt.EventID, 24*time.Hour)
	return nil
}

func (h *EventHandler) handlePaymentFailed(ctx context.Context, msg *messaging.Message) error {
	var evt events.PaymentFailedEvent
	if err := json.Unmarshal(msg.Value, &evt); err != nil {
		return err
	}

	if processed, _ := h.idemStore.IsProcessed(ctx, evt.EventID); processed {
		h.logger.Info("event already processed", zap.String("eventId", evt.EventID))
		return nil
	}

	if err := h.orderService.HandlePaymentFailed(ctx, evt); err != nil {
		return err
	}

	_ = h.idemStore.Reserve(ctx, evt.EventID, 24*time.Hour)
	return nil
}

func (h *EventHandler) handleStockReserved(ctx context.Context, msg *messaging.Message) error {
	var evt events.StockReservedEvent
	if err := json.Unmarshal(msg.Value, &evt); err != nil {
		return err
	}

	if processed, _ := h.idemStore.IsProcessed(ctx, evt.EventID); processed {
		h.logger.Info("event already processed", zap.String("eventId", evt.EventID))
		return nil
	}

	if err := h.orderService.HandleStockReserved(ctx, evt); err != nil {
		return err
	}

	_ = h.idemStore.Reserve(ctx, evt.EventID, 24*time.Hour)
	return nil
}

func (h *EventHandler) handleStockReservationFailed(ctx context.Context, msg *messaging.Message) error {
	var evt events.StockReservationFailedEvent
	if err := json.Unmarshal(msg.Value, &evt); err != nil {
		return err
	}

	if processed, _ := h.idemStore.IsProcessed(ctx, evt.EventID); processed {
		h.logger.Info("event already processed", zap.String("eventId", evt.EventID))
		return nil
	}

	if err := h.orderService.HandleStockReservationFailed(ctx, evt); err != nil {
		return err
	}

	_ = h.idemStore.Reserve(ctx, evt.EventID, 24*time.Hour)
	return nil
}

func (h *EventHandler) handleDeliveryStarted(ctx context.Context, msg *messaging.Message) error {
	var evt events.DeliveryStartedEvent
	if err := json.Unmarshal(msg.Value, &evt); err != nil {
		return err
	}

	if processed, _ := h.idemStore.IsProcessed(ctx, evt.EventID); processed {
		h.logger.Info("event already processed", zap.String("eventId", evt.EventID))
		return nil
	}

	if err := h.orderService.HandleDeliveryStarted(ctx, evt); err != nil {
		return err
	}

	_ = h.idemStore.Reserve(ctx, evt.EventID, 24*time.Hour)
	return nil
}

func (h *EventHandler) handleDeliveryFailed(ctx context.Context, msg *messaging.Message) error {
	var evt events.DeliveryFailedEvent
	if err := json.Unmarshal(msg.Value, &evt); err != nil {
		return err
	}

	if processed, _ := h.idemStore.IsProcessed(ctx, evt.EventID); processed {
		h.logger.Info("event already processed", zap.String("eventId", evt.EventID))
		return nil
	}

	if err := h.orderService.HandleDeliveryFailed(ctx, evt); err != nil {
		return err
	}

	_ = h.idemStore.Reserve(ctx, evt.EventID, 24*time.Hour)
	return nil
}

