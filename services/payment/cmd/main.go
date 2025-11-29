package main

import (
	"context"
	"database/sql"
	"encoding/json"
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

	"github.com/kyungseok/msa-saga-go-practical/common/idempotency"
	"github.com/kyungseok/msa-saga-go-practical/common/logger"
	"github.com/kyungseok/msa-saga-go-practical/common/messaging"
	"github.com/kyungseok/msa-saga-go-practical/services/payment/internal/handler"
	"github.com/kyungseok/msa-saga-go-practical/services/payment/internal/repository"
	"github.com/kyungseok/msa-saga-go-practical/services/payment/internal/service"
)

func main() {
	// Logger 초기화
	log, err := logger.NewLogger("payment-service", true)
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
	paymentRepo := repository.NewPaymentRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)

	// Service 초기화
	paymentService := service.NewPaymentService(db, paymentRepo, outboxRepo, log)

	// Idempotency Store 초기화
	idemStore := idempotency.NewRedisStore(redisClient, "payment-service")

	// Event Handler 초기화
	eventHandler := handler.NewEventHandler(paymentService, idemStore, log)

	// Kafka Consumer 초기화
	consumer, err := messaging.NewKafkaConsumer(config.KafkaBrokers, "payment-service-group", log)
	if err != nil {
		log.Fatal("failed to create kafka consumer", zap.Error(err))
	}
	defer consumer.Close()

	// 구독할 토픽 설정
	topics := []string{
		"order.created.v1",
		"stock.reservation_failed.v1",
	}

	if err := consumer.Subscribe(topics, eventHandler.HandleMessage); err != nil {
		log.Fatal("failed to subscribe to topics", zap.Error(err))
	}
	log.Info("subscribed to kafka topics", zap.Strings("topics", topics))

	// Outbox Worker 시작
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go startOutboxWorker(ctx, db, publisher, log)
	log.Info("outbox worker started")

	// HTTP Server 시작 (헬스 체크용)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

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

func startOutboxWorker(ctx context.Context, db *sql.DB, publisher messaging.Publisher, logger *zap.Logger) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rows, err := db.QueryContext(ctx, `
				SELECT id, event_type, payload FROM outbox_events
				WHERE status = 'PENDING' ORDER BY created_at LIMIT 100
			`)
			if err != nil {
				continue
			}

			for rows.Next() {
				var id int64
				var eventType string
				var payload []byte
				if err := rows.Scan(&id, &eventType, &payload); err != nil {
					continue
				}

				if err := publisher.Publish(ctx, eventType, "", json.RawMessage(payload)); err != nil {
					logger.Error("failed to publish", zap.Error(err))
					continue
				}

				db.ExecContext(ctx, `UPDATE outbox_events SET status = 'SENT', sent_at = NOW() WHERE id = $1`, id)
			}
			rows.Close()
		}
	}
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
		DBDSN:        getEnv("DB_DSN", "postgres://payment:payment@localhost:54322/payment_db?sslmode=disable"),
		RedisAddr:    getEnv("REDIS_ADDR", "localhost:6379"),
		KafkaBrokers: strings.Split(getEnv("KAFKA_BROKERS", "localhost:9093"), ","),
		ServicePort:  getEnv("SERVICE_PORT", "8002"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
