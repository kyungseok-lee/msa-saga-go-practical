package domain

import "time"

// PaymentStatus 결제 상태
type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "PENDING"
	PaymentStatusCompleted PaymentStatus = "COMPLETED"
	PaymentStatusFailed    PaymentStatus = "FAILED"
	PaymentStatusRefunded  PaymentStatus = "REFUNDED"
)

// Payment 결제 도메인 모델
type Payment struct {
	ID                 int64
	OrderID            int64
	Amount             int64
	PaymentType        string
	Status             PaymentStatus
	IdempotencyKey     string
	PaymentGatewayTxID string
	Reason             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// CanRefund 환불 가능 여부 확인
func (p *Payment) CanRefund() bool {
	return p.Status == PaymentStatusCompleted
}

// Refund 환불 처리
func (p *Payment) Refund(reason string) bool {
	if !p.CanRefund() {
		return false
	}
	p.Status = PaymentStatusRefunded
	p.Reason = reason
	p.UpdatedAt = time.Now()
	return true
}

