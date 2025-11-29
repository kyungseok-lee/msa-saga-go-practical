package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/kyungseok/msa-saga-go-practical/services/order/internal/service"
	"go.uber.org/zap"
)

// HTTPHandler HTTP 핸들러
type HTTPHandler struct {
	orderService service.OrderService
	logger       *zap.Logger
}

// NewHTTPHandler HTTP 핸들러 생성
func NewHTTPHandler(orderService service.OrderService, logger *zap.Logger) *HTTPHandler {
	return &HTTPHandler{
		orderService: orderService,
		logger:       logger,
	}
}

// CreateOrderRequest 주문 생성 요청
type CreateOrderRequest struct {
	UserID         int64  `json:"userId"`
	Amount         int64  `json:"amount"`
	Quantity       int    `json:"quantity"`
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
}

// CreateOrderResponse 주문 생성 응답
type CreateOrderResponse struct {
	OrderID int64  `json:"orderId"`
	Status  string `json:"status"`
}

// ErrorResponse 에러 응답
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// CreateOrder 주문 생성 API
func (h *HTTPHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed", "")
		return
	}

	var req CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body", "")
		return
	}

	// IdempotencyKey가 없으면 생성
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = uuid.New().String()
	}

	cmd := service.CreateOrderCommand{
		UserID:         req.UserID,
		Amount:         req.Amount,
		Quantity:       req.Quantity,
		IdempotencyKey: req.IdempotencyKey,
	}

	result, err := h.orderService.CreateOrder(r.Context(), cmd)
	if err != nil {
		h.logger.Error("failed to create order", zap.Error(err))
		h.respondError(w, http.StatusInternalServerError, err.Error(), "")
		return
	}

	h.respondJSON(w, http.StatusCreated, CreateOrderResponse{
		OrderID: result.OrderID,
		Status:  string(result.Status),
	})
}

// GetOrder 주문 조회 API
func (h *HTTPHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.respondError(w, http.StatusMethodNotAllowed, "method not allowed", "")
		return
	}

	// URL에서 orderID 파싱 (예: /orders/123)
	orderIDStr := r.URL.Path[len("/orders/"):]
	orderID, err := strconv.ParseInt(orderIDStr, 10, 64)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid order ID", "")
		return
	}

	order, err := h.orderService.GetOrder(r.Context(), orderID)
	if err != nil {
		h.logger.Error("failed to get order", zap.Error(err))
		h.respondError(w, http.StatusNotFound, "order not found", "")
		return
	}

	h.respondJSON(w, http.StatusOK, order)
}

// HealthCheck 헬스 체크 API
func (h *HTTPHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	h.respondJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

func (h *HTTPHandler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *HTTPHandler) respondError(w http.ResponseWriter, status int, message string, code string) {
	h.respondJSON(w, status, ErrorResponse{
		Error: message,
		Code:  code,
	})
}

