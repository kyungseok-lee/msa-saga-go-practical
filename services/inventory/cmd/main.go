package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/kyungseok/msa-saga-go-practical/common/events"
	"github.com/kyungseok/msa-saga-go-practical/common/idempotency"
	"github.com/kyungseok/msa-saga-go-practical/common/logger"
	"github.com/kyungseok/msa-saga-go-practical/common/messaging"
	"github.com/kyungseok/msa-saga-go-practical/services/inventory/internal/service"
)

func main() {
	log, _ := logger.NewLogger("inventory-service", true)
	defer log.Sync()

	config := loadConfig()

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

	redisClient := redis.NewClient(&redis.Options{Addr: config.RedisAddr})
	defer redisClient.Close()

	publisher, err := messaging.NewKafkaPublisher(config.KafkaBrokers, log)
	if err != nil {
		log.Fatal("failed to create kafka publisher", zap.Error(err))
	}
	defer publisher.Close()

	inventoryService := service.NewInventoryService(db, log)
	idemStore := idempotency.NewRedisStore(redisClient, "inventory-service")

	consumer, err := messaging.NewKafkaConsumer(config.KafkaBrokers, "inventory-service-group", log)
	if err != nil {
		log.Fatal("failed to create kafka consumer", zap.Error(err))
	}
	defer consumer.Close()

	topics := []string{"payment.completed.v1", "payment.refunded.v1"}

	// Event Handler
	eventHandler := func(ctx context.Context, msg *messaging.Message) error {
		log.Info("received message", zap.String("topic", msg.Topic))

		switch events.EventType(msg.Topic) {
		case events.EventPaymentCompleted:
			var evt events.PaymentCompletedEvent
			if err := json.Unmarshal(msg.Value, &evt); err != nil {
				return err
			}
			if processed, _ := idemStore.IsProcessed(ctx, evt.EventID); processed {
				return nil
			}
			if err := inventoryService.HandlePaymentCompleted(ctx, evt); err != nil {
				return err
			}
			_ = idemStore.Reserve(ctx, evt.EventID, 24*time.Hour)

		case events.EventPaymentRefunded:
			var evt events.PaymentRefundedEvent
			if err := json.Unmarshal(msg.Value, &evt); err != nil {
				return err
			}
			if processed, _ := idemStore.IsProcessed(ctx, evt.EventID); processed {
				return nil
			}
			if err := inventoryService.HandlePaymentRefunded(ctx, evt); err != nil {
				return err
			}
			_ = idemStore.Reserve(ctx, evt.EventID, 24*time.Hour)
		}
		return nil
	}

	if err := consumer.Subscribe(topics, eventHandler); err != nil {
		log.Fatal("failed to subscribe", zap.Error(err))
	}
	log.Info("subscribed to kafka topics", zap.Strings("topics", topics))

	// Outbox Worker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go startOutboxWorker(ctx, db, publisher, log)

	// HTTP Server
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	server := &http.Server{Addr: ":" + config.ServicePort, Handler: mux}

	go func() {
		log.Info("http server starting", zap.String("port", config.ServicePort))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("http server failed", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("server forced to shutdown", zap.Error(err))
	}

	cancel()
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

type Config struct {
	DBDSN        string
	RedisAddr    string
	KafkaBrokers []string
	ServicePort  string
}

func loadConfig() Config {
	return Config{
		DBDSN:        getEnv("DB_DSN", "postgres://inventory:inventory@localhost:54323/inventory_db?sslmode=disable"),
		RedisAddr:    getEnv("REDIS_ADDR", "localhost:6379"),
		KafkaBrokers: strings.Split(getEnv("KAFKA_BROKERS", "localhost:9093"), ","),
		ServicePort:  getEnv("SERVICE_PORT", "8003"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
