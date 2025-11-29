package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// OutboxEvent Outbox 이벤트
type OutboxEvent struct {
	ID            int64
	AggregateType string
	AggregateID   int64
	EventType     string
	Payload       json.RawMessage
	Status        string
	CreatedAt     time.Time
	SentAt        *time.Time
}

// OutboxRepository Outbox 레포지토리 인터페이스
type OutboxRepository interface {
	Insert(ctx context.Context, event *OutboxEvent) error
	InsertTx(ctx context.Context, tx *sql.Tx, event *OutboxEvent) error
	FindPending(ctx context.Context, limit int) ([]*OutboxEvent, error)
	MarkSent(ctx context.Context, id int64) error
}

type outboxRepository struct {
	db *sql.DB
}

// NewOutboxRepository Outbox 레포지토리 생성
func NewOutboxRepository(db *sql.DB) OutboxRepository {
	return &outboxRepository{db: db}
}

// Insert Outbox 이벤트 삽입
func (r *outboxRepository) Insert(ctx context.Context, event *OutboxEvent) error {
	query := `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`

	err := r.db.QueryRowContext(
		ctx,
		query,
		event.AggregateType,
		event.AggregateID,
		event.EventType,
		event.Payload,
		event.Status,
		event.CreatedAt,
	).Scan(&event.ID)

	if err != nil {
		return fmt.Errorf("failed to insert outbox event: %w", err)
	}

	return nil
}

// InsertTx 트랜잭션 내에서 Outbox 이벤트 삽입
func (r *outboxRepository) InsertTx(ctx context.Context, tx *sql.Tx, event *OutboxEvent) error {
	query := `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`

	err := tx.QueryRowContext(
		ctx,
		query,
		event.AggregateType,
		event.AggregateID,
		event.EventType,
		event.Payload,
		event.Status,
		event.CreatedAt,
	).Scan(&event.ID)

	if err != nil {
		return fmt.Errorf("failed to insert outbox event: %w", err)
	}

	return nil
}

// FindPending 전송 대기 중인 이벤트 조회
func (r *outboxRepository) FindPending(ctx context.Context, limit int) ([]*OutboxEvent, error) {
	query := `
		SELECT id, aggregate_type, aggregate_id, event_type, payload, status, created_at
		FROM outbox_events
		WHERE status = 'PENDING'
		ORDER BY created_at ASC
		LIMIT $1
	`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to find pending events: %w", err)
	}
	defer rows.Close()

	var events []*OutboxEvent
	for rows.Next() {
		event := &OutboxEvent{}
		err := rows.Scan(
			&event.ID,
			&event.AggregateType,
			&event.AggregateID,
			&event.EventType,
			&event.Payload,
			&event.Status,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan outbox event: %w", err)
		}
		events = append(events, event)
	}

	return events, nil
}

// MarkSent 이벤트를 전송 완료로 표시
func (r *outboxRepository) MarkSent(ctx context.Context, id int64) error {
	query := `
		UPDATE outbox_events
		SET status = 'SENT', sent_at = NOW()
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to mark event as sent: %w", err)
	}

	return nil
}
