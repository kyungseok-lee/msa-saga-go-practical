package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kyungseok/msa-saga-go-examples/services/payment/internal/domain"
	"github.com/lib/pq"
)

// PaymentRepository 결제 레포지토리 인터페이스
type PaymentRepository interface {
	Create(ctx context.Context, payment *domain.Payment) error
	FindByID(ctx context.Context, id int64) (*domain.Payment, error)
	FindByOrderID(ctx context.Context, orderID int64) (*domain.Payment, error)
	FindByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error)
	UpdateStatus(ctx context.Context, id int64, status domain.PaymentStatus, reason string) error
}

type paymentRepository struct {
	db *sql.DB
}

// NewPaymentRepository 결제 레포지토리 생성
func NewPaymentRepository(db *sql.DB) PaymentRepository {
	return &paymentRepository{db: db}
}

// Create 결제 생성
func (r *paymentRepository) Create(ctx context.Context, payment *domain.Payment) error {
	query := `
		INSERT INTO payments (order_id, amount, payment_type, status, idempotency_key, payment_gateway_tx_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`

	err := r.db.QueryRowContext(
		ctx,
		query,
		payment.OrderID,
		payment.Amount,
		payment.PaymentType,
		payment.Status,
		payment.IdempotencyKey,
		payment.PaymentGatewayTxID,
		payment.CreatedAt,
		payment.UpdatedAt,
	).Scan(&payment.ID)

	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return fmt.Errorf("duplicate idempotency key: %w", err)
		}
		return fmt.Errorf("failed to create payment: %w", err)
	}

	return nil
}

// FindByID ID로 결제 조회
func (r *paymentRepository) FindByID(ctx context.Context, id int64) (*domain.Payment, error) {
	query := `
		SELECT id, order_id, amount, payment_type, status, idempotency_key, payment_gateway_tx_id, reason, created_at, updated_at
		FROM payments
		WHERE id = $1
	`

	payment := &domain.Payment{}
	var reason sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&payment.ID,
		&payment.OrderID,
		&payment.Amount,
		&payment.PaymentType,
		&payment.Status,
		&payment.IdempotencyKey,
		&payment.PaymentGatewayTxID,
		&reason,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("payment not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find payment: %w", err)
	}

	if reason.Valid {
		payment.Reason = reason.String
	}

	return payment, nil
}

// FindByOrderID OrderID로 결제 조회
func (r *paymentRepository) FindByOrderID(ctx context.Context, orderID int64) (*domain.Payment, error) {
	query := `
		SELECT id, order_id, amount, payment_type, status, idempotency_key, payment_gateway_tx_id, reason, created_at, updated_at
		FROM payments
		WHERE order_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	payment := &domain.Payment{}
	var reason sql.NullString

	err := r.db.QueryRowContext(ctx, query, orderID).Scan(
		&payment.ID,
		&payment.OrderID,
		&payment.Amount,
		&payment.PaymentType,
		&payment.Status,
		&payment.IdempotencyKey,
		&payment.PaymentGatewayTxID,
		&reason,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("payment not found for order: %d", orderID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find payment: %w", err)
	}

	if reason.Valid {
		payment.Reason = reason.String
	}

	return payment, nil
}

// FindByIdempotencyKey 멱등성 키로 결제 조회
func (r *paymentRepository) FindByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error) {
	query := `
		SELECT id, order_id, amount, payment_type, status, idempotency_key, payment_gateway_tx_id, reason, created_at, updated_at
		FROM payments
		WHERE idempotency_key = $1
	`

	payment := &domain.Payment{}
	var reason sql.NullString

	err := r.db.QueryRowContext(ctx, query, key).Scan(
		&payment.ID,
		&payment.OrderID,
		&payment.Amount,
		&payment.PaymentType,
		&payment.Status,
		&payment.IdempotencyKey,
		&payment.PaymentGatewayTxID,
		&reason,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("payment not found with idempotency key: %s", key)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find payment: %w", err)
	}

	if reason.Valid {
		payment.Reason = reason.String
	}

	return payment, nil
}

// UpdateStatus 결제 상태 업데이트
func (r *paymentRepository) UpdateStatus(ctx context.Context, id int64, status domain.PaymentStatus, reason string) error {
	query := `
		UPDATE payments
		SET status = $1, reason = $2, updated_at = NOW()
		WHERE id = $3
	`

	_, err := r.db.ExecContext(ctx, query, status, reason, id)
	if err != nil {
		return fmt.Errorf("failed to update payment status: %w", err)
	}

	return nil
}
