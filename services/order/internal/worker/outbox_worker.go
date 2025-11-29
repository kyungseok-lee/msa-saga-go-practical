package worker

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kyungseok/msa-saga-go-examples/common/messaging"
	"github.com/kyungseok/msa-saga-go-examples/services/order/internal/repository"
	"go.uber.org/zap"
)

// OutboxWorker Outbox 패턴 워커
type OutboxWorker struct {
	outboxRepo repository.OutboxRepository
	publisher  messaging.Publisher
	logger     *zap.Logger
	interval   time.Duration
}

// NewOutboxWorker Outbox 워커 생성
func NewOutboxWorker(
	outboxRepo repository.OutboxRepository,
	publisher messaging.Publisher,
	logger *zap.Logger,
	interval time.Duration,
) *OutboxWorker {
	return &OutboxWorker{
		outboxRepo: outboxRepo,
		publisher:  publisher,
		logger:     logger,
		interval:   interval,
	}
}

// Start 워커 시작
func (w *OutboxWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.logger.Info("outbox worker started", zap.Duration("interval", w.interval))

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("outbox worker stopped")
			return
		case <-ticker.C:
			if err := w.process(ctx); err != nil {
				w.logger.Error("failed to process outbox events", zap.Error(err))
			}
		}
	}
}

func (w *OutboxWorker) process(ctx context.Context) error {
	// Pending 상태의 이벤트 조회
	events, err := w.outboxRepo.FindPending(ctx, 100)
	if err != nil {
		return err
	}

	if len(events) == 0 {
		return nil
	}

	w.logger.Info("processing outbox events", zap.Int("count", len(events)))

	for _, event := range events {
		if err := w.publishEvent(ctx, event); err != nil {
			w.logger.Error("failed to publish event",
				zap.Int64("eventId", event.ID),
				zap.String("eventType", event.EventType),
				zap.Error(err))
			continue
		}

		// 전송 완료 표시
		if err := w.outboxRepo.MarkSent(ctx, event.ID); err != nil {
			w.logger.Error("failed to mark event as sent",
				zap.Int64("eventId", event.ID),
				zap.Error(err))
		}
	}

	return nil
}

func (w *OutboxWorker) publishEvent(ctx context.Context, event *repository.OutboxEvent) error {
	// Event payload에서 key 추출 (orderId 기준)
	var payload map[string]interface{}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	// OrderID를 키로 사용 (파티셔닝)
	key := ""
	if orderID, ok := payload["orderId"]; ok {
		if oid, ok := orderID.(float64); ok {
			key = string(rune(int64(oid)))
		}
	}

	// Kafka 토픽으로 발행
	return w.publisher.Publish(ctx, event.EventType, key, json.RawMessage(event.Payload))
}
