package domain

import "time"

// OrderStatus 주문 상태
type OrderStatus string

const (
	OrderStatusPending            OrderStatus = "PENDING"
	OrderStatusPaymentProcessing  OrderStatus = "PAYMENT_PROCESSING"
	OrderStatusStockReserving     OrderStatus = "STOCK_RESERVING"
	OrderStatusDeliveryPreparing  OrderStatus = "DELIVERY_PREPARING"
	OrderStatusCompleted          OrderStatus = "COMPLETED"
	OrderStatusCanceled           OrderStatus = "CANCELED"
	OrderStatusFailed             OrderStatus = "FAILED"
)

// Order 주문 도메인 모델
type Order struct {
	ID              int64
	UserID          int64
	Amount          int64
	Quantity        int
	Status          OrderStatus
	Version         int64
	IdempotencyKey  string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// CanTransitionTo 상태 전이 가능 여부 확인
func (o *Order) CanTransitionTo(newStatus OrderStatus) bool {
	transitions := map[OrderStatus][]OrderStatus{
		OrderStatusPending: {
			OrderStatusPaymentProcessing,
			OrderStatusCanceled,
			OrderStatusFailed,
		},
		OrderStatusPaymentProcessing: {
			OrderStatusStockReserving,
			OrderStatusCanceled,
			OrderStatusFailed,
		},
		OrderStatusStockReserving: {
			OrderStatusDeliveryPreparing,
			OrderStatusCanceled,
			OrderStatusFailed,
		},
		OrderStatusDeliveryPreparing: {
			OrderStatusCompleted,
			OrderStatusCanceled,
			OrderStatusFailed,
		},
	}

	allowedTransitions, exists := transitions[o.Status]
	if !exists {
		return false
	}

	for _, allowed := range allowedTransitions {
		if allowed == newStatus {
			return true
		}
	}

	return false
}

// TransitionTo 상태 전이 (Semantic Lock 패턴)
func (o *Order) TransitionTo(newStatus OrderStatus) bool {
	if !o.CanTransitionTo(newStatus) {
		return false
	}
	o.Status = newStatus
	o.UpdatedAt = time.Now()
	return true
}

