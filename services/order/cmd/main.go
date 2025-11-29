package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/kyungseok/msa-saga-go-examples/common/idempotency"
	"github.com/kyungseok/msa-saga-go-examples/common/logger"
	"github.com/kyungseok/msa-saga-go-examples/common/messaging"
	"github.com/kyungseok/msa-saga-go-examples/services/order/internal/handler"
	"github.com/kyungseok/msa-saga-go-examples/services/order/internal/repository"
	"github.com/kyungseok/msa-saga-go-examples/services/order/internal/service"
	"github.com/kyungseok/msa-saga-go-examples/services/order/internal/worker"
)

func main() {
	// Logger 초기화
	log, err := logger.NewLogger("order-service", true)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize logger: %v", err))
	}
	defer log.Sync()

	// Config 로드
	config := loadConfig()

	// PostgreSQL 연결
	db, err := sql.Open("postgres", config.DBDSN)
	if err != nil {
		log.Fatal("failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Fatal("failed to ping database", zap.Error(err))
	}
	log.Info("connected to database")

	// Redis 연결
	redisClient := redis.NewClient(&redis.Options{
		Addr: config.RedisAddr,
	})
	defer redisClient.Close()

	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		log.Fatal("failed to connect to redis", zap.Error(err))
	}
	log.Info("connected to redis")

	// Kafka Producer 초기화
	publisher, err := messaging.NewKafkaPublisher(config.KafkaBrokers, log)
	if err != nil {
		log.Fatal("failed to create kafka publisher", zap.Error(err))
	}
	defer publisher.Close()
	log.Info("kafka publisher initialized")

	// Repository 초기화
	orderRepo := repository.NewOrderRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)

	// Service 초기화
	orderService := service.NewOrderService(db, orderRepo, outboxRepo, log)

	// Idempotency Store 초기화
	idemStore := idempotency.NewRedisStore(redisClient, "order-service")

	// Event Handler 초기화
	eventHandler := handler.NewEventHandler(orderService, idemStore, log)

	// Kafka Consumer 초기화
	consumer, err := messaging.NewKafkaConsumer(config.KafkaBrokers, "order-service-group", log)
	if err != nil {
		log.Fatal("failed to create kafka consumer", zap.Error(err))
	}
	defer consumer.Close()

	// 구독할 토픽 설정
	topics := []string{
		"payment.completed.v1",
		"payment.failed.v1",
		"stock.reserved.v1",
		"stock.reservation_failed.v1",
		"delivery.started.v1",
		"delivery.failed.v1",
	}

	if err := consumer.Subscribe(topics, eventHandler.HandleMessage); err != nil {
		log.Fatal("failed to subscribe to topics", zap.Error(err))
	}
	log.Info("subscribed to kafka topics", zap.Strings("topics", topics))

	// Outbox Worker 시작
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outboxWorker := worker.NewOutboxWorker(outboxRepo, publisher, log, 1*time.Second)
	go outboxWorker.Start(ctx)
	log.Info("outbox worker started")

	// HTTP Server 시작
	httpHandler := handler.NewHTTPHandler(orderService, log)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", httpHandler.HealthCheck)
	mux.HandleFunc("/orders", httpHandler.CreateOrder)
	mux.HandleFunc("/orders/", httpHandler.GetOrder)

	server := &http.Server{
		Addr:    ":" + config.ServicePort,
		Handler: mux,
	}

	go func() {
		log.Info("http server starting", zap.String("port", config.ServicePort))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("http server failed", zap.Error(err))
		}
	}()

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("server forced to shutdown", zap.Error(err))
	}

	cancel() // outbox worker 종료
	log.Info("server stopped")
}

// Config 설정 구조체
type Config struct {
	DBDSN        string
	RedisAddr    string
	KafkaBrokers []string
	ServicePort  string
}

func loadConfig() Config {
	return Config{
		DBDSN:        getEnv("DB_DSN", "postgres://order:order@localhost:54321/order_db?sslmode=disable"),
		RedisAddr:    getEnv("REDIS_ADDR", "localhost:6379"),
		KafkaBrokers: strings.Split(getEnv("KAFKA_BROKERS", "localhost:9093"), ","),
		ServicePort:  getEnv("SERVICE_PORT", "8001"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
