package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kyungseok/msa-saga-go-practical/services/order/internal/domain"
	"github.com/lib/pq"
)

// OrderRepository 주문 레포지토리 인터페이스
type OrderRepository interface {
	Create(ctx context.Context, order *domain.Order) error
	FindByID(ctx context.Context, id int64) (*domain.Order, error)
	FindByIdempotencyKey(ctx context.Context, key string) (*domain.Order, error)
	UpdateStatus(ctx context.Context, id int64, status domain.OrderStatus, version int64) error
	UpdateStatusWithVersion(ctx context.Context, id int64, status domain.OrderStatus, currentVersion int64) (bool, error)
}

type orderRepository struct {
	db *sql.DB
}

// NewOrderRepository 주문 레포지토리 생성
func NewOrderRepository(db *sql.DB) OrderRepository {
	return &orderRepository{db: db}
}

// Create 주문 생성
func (r *orderRepository) Create(ctx context.Context, order *domain.Order) error {
	query := `
		INSERT INTO orders (user_id, amount, quantity, status, idempotency_key, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, version
	`

	err := r.db.QueryRowContext(
		ctx,
		query,
		order.UserID,
		order.Amount,
		order.Quantity,
		order.Status,
		order.IdempotencyKey,
		order.CreatedAt,
		order.UpdatedAt,
	).Scan(&order.ID, &order.Version)

	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			// Unique constraint violation
			return fmt.Errorf("duplicate idempotency key: %w", err)
		}
		return fmt.Errorf("failed to create order: %w", err)
	}

	return nil
}

// FindByID ID로 주문 조회
func (r *orderRepository) FindByID(ctx context.Context, id int64) (*domain.Order, error) {
	query := `
		SELECT id, user_id, amount, quantity, status, version, idempotency_key, created_at, updated_at
		FROM orders
		WHERE id = $1
	`

	order := &domain.Order{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&order.ID,
		&order.UserID,
		&order.Amount,
		&order.Quantity,
		&order.Status,
		&order.Version,
		&order.IdempotencyKey,
		&order.CreatedAt,
		&order.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("order not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find order: %w", err)
	}

	return order, nil
}

// FindByIdempotencyKey 멱등성 키로 주문 조회
func (r *orderRepository) FindByIdempotencyKey(ctx context.Context, key string) (*domain.Order, error) {
	query := `
		SELECT id, user_id, amount, quantity, status, version, idempotency_key, created_at, updated_at
		FROM orders
		WHERE idempotency_key = $1
	`

	order := &domain.Order{}
	err := r.db.QueryRowContext(ctx, query, key).Scan(
		&order.ID,
		&order.UserID,
		&order.Amount,
		&order.Quantity,
		&order.Status,
		&order.Version,
		&order.IdempotencyKey,
		&order.CreatedAt,
		&order.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("order not found with idempotency key: %s", key)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find order: %w", err)
	}

	return order, nil
}

// UpdateStatus 주문 상태 업데이트
func (r *orderRepository) UpdateStatus(ctx context.Context, id int64, status domain.OrderStatus, version int64) error {
	query := `
		UPDATE orders
		SET status = $1, version = $2, updated_at = NOW()
		WHERE id = $3
	`

	_, err := r.db.ExecContext(ctx, query, status, version+1, id)
	if err != nil {
		return fmt.Errorf("failed to update order status: %w", err)
	}

	return nil
}

// UpdateStatusWithVersion Optimistic Lock을 사용한 상태 업데이트
func (r *orderRepository) UpdateStatusWithVersion(ctx context.Context, id int64, status domain.OrderStatus, currentVersion int64) (bool, error) {
	query := `
		UPDATE orders
		SET status = $1, version = version + 1, updated_at = NOW()
		WHERE id = $2 AND version = $3
	`

	result, err := r.db.ExecContext(ctx, query, status, id, currentVersion)
	if err != nil {
		return false, fmt.Errorf("failed to update order status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected > 0, nil
}

