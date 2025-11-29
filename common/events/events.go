package events

import "time"

// EventType 이벤트 타입 정의
type EventType string

const (
	// Order Events
	EventOrderCreated   EventType = "order.created.v1"
	EventOrderCompleted EventType = "order.completed.v1"
	EventOrderCanceled  EventType = "order.canceled.v1"
	EventOrderFailed    EventType = "order.failed.v1"

	// Payment Events
	EventPaymentCompleted EventType = "payment.completed.v1"
	EventPaymentFailed    EventType = "payment.failed.v1"
	EventPaymentRefunded  EventType = "payment.refunded.v1"

	// Inventory Events
	EventStockReserved          EventType = "stock.reserved.v1"
	EventStockReservationFailed EventType = "stock.reservation_failed.v1"
	EventStockRestored          EventType = "stock.restored.v1"

	// Delivery Events
	EventDeliveryStarted EventType = "delivery.started.v1"
	EventDeliveryFailed  EventType = "delivery.failed.v1"
)

// BaseEvent 모든 이벤트의 기본 구조
type BaseEvent struct {
	EventID       string    `json:"eventId"`
	EventType     EventType `json:"eventType"`
	SchemaVersion int       `json:"schemaVersion"`
	OccurredAt    time.Time `json:"occurredAt"`
	CorrelationID string    `json:"correlationId"` // SAGA ID로 사용
}

// OrderCreatedEvent 주문 생성 이벤트
type OrderCreatedEvent struct {
	BaseEvent
	OrderID  int64 `json:"orderId"`
	UserID   int64 `json:"userId"`
	Amount   int64 `json:"amount"`
	Quantity int   `json:"quantity"`
}

// OrderCompletedEvent 주문 완료 이벤트
type OrderCompletedEvent struct {
	BaseEvent
	OrderID int64 `json:"orderId"`
}

// OrderCanceledEvent 주문 취소 이벤트
type OrderCanceledEvent struct {
	BaseEvent
	OrderID int64  `json:"orderId"`
	Reason  string `json:"reason"`
}

// OrderFailedEvent 주문 실패 이벤트
type OrderFailedEvent struct {
	BaseEvent
	OrderID int64  `json:"orderId"`
	Reason  string `json:"reason"`
}

// PaymentCompletedEvent 결제 완료 이벤트
type PaymentCompletedEvent struct {
	BaseEvent
	OrderID     int64  `json:"orderId"`
	PaymentID   int64  `json:"paymentId"`
	Amount      int64  `json:"amount"`
	PaymentType string `json:"paymentType"`
}

// PaymentFailedEvent 결제 실패 이벤트
type PaymentFailedEvent struct {
	BaseEvent
	OrderID int64  `json:"orderId"`
	Reason  string `json:"reason"`
}

// PaymentRefundedEvent 결제 환불 이벤트
type PaymentRefundedEvent struct {
	BaseEvent
	OrderID   int64 `json:"orderId"`
	PaymentID int64 `json:"paymentId"`
	Amount    int64 `json:"amount"`
}

// StockReservedEvent 재고 예약 이벤트
type StockReservedEvent struct {
	BaseEvent
	OrderID       int64 `json:"orderId"`
	ReservationID int64 `json:"reservationId"`
	Quantity      int   `json:"quantity"`
}

// StockReservationFailedEvent 재고 예약 실패 이벤트
type StockReservationFailedEvent struct {
	BaseEvent
	OrderID  int64  `json:"orderId"`
	Quantity int    `json:"quantity"`
	Reason   string `json:"reason"`
}

// StockRestoredEvent 재고 복구 이벤트
type StockRestoredEvent struct {
	BaseEvent
	OrderID       int64 `json:"orderId"`
	ReservationID int64 `json:"reservationId"`
	Quantity      int   `json:"quantity"`
}

// DeliveryStartedEvent 배송 시작 이벤트
type DeliveryStartedEvent struct {
	BaseEvent
	OrderID    int64  `json:"orderId"`
	DeliveryID int64  `json:"deliveryId"`
	Address    string `json:"address"`
}

// DeliveryFailedEvent 배송 실패 이벤트
type DeliveryFailedEvent struct {
	BaseEvent
	OrderID int64  `json:"orderId"`
	Reason  string `json:"reason"`
}
