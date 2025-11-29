package errors

import "fmt"

// ErrorCode 에러 코드 정의
type ErrorCode string

const (
	// Business Errors
	ErrCodePaymentDeclined     ErrorCode = "PAYMENT_DECLINED"
	ErrCodeOutOfStock          ErrorCode = "OUT_OF_STOCK"
	ErrCodeInsufficientBalance ErrorCode = "INSUFFICIENT_BALANCE"
	ErrCodeInvalidOrder        ErrorCode = "INVALID_ORDER"
	ErrCodeOrderNotFound       ErrorCode = "ORDER_NOT_FOUND"
	ErrCodeDuplicateRequest    ErrorCode = "DUPLICATE_REQUEST"

	// Technical Errors
	ErrCodeDatabaseError      ErrorCode = "DATABASE_ERROR"
	ErrCodeNetworkError       ErrorCode = "NETWORK_ERROR"
	ErrCodeTimeoutError       ErrorCode = "TIMEOUT_ERROR"
	ErrCodeSerializationError ErrorCode = "SERIALIZATION_ERROR"
	ErrCodeUnknownError       ErrorCode = "UNKNOWN_ERROR"
)

// DomainError 도메인 에러 구조체
type DomainError struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *DomainError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *DomainError) Unwrap() error {
	return e.Cause
}

// New 새로운 도메인 에러 생성
func New(code ErrorCode, message string) *DomainError {
	return &DomainError{
		Code:    code,
		Message: message,
	}
}

// Wrap 기존 에러를 래핑한 도메인 에러 생성
func Wrap(code ErrorCode, message string, cause error) *DomainError {
	return &DomainError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// IsRetryable 재시도 가능한 에러인지 판단
func IsRetryable(err error) bool {
	if domainErr, ok := err.(*DomainError); ok {
		switch domainErr.Code {
		case ErrCodeDatabaseError, ErrCodeNetworkError, ErrCodeTimeoutError:
			return true
		}
	}
	return false
}

// IsBusinessError 비즈니스 에러인지 판단 (재시도 불필요)
func IsBusinessError(err error) bool {
	if domainErr, ok := err.(*DomainError); ok {
		switch domainErr.Code {
		case ErrCodePaymentDeclined, ErrCodeOutOfStock, ErrCodeInsufficientBalance,
			ErrCodeInvalidOrder, ErrCodeOrderNotFound, ErrCodeDuplicateRequest:
			return true
		}
	}
	return false
}
