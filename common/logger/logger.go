package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger 새로운 로거 생성
func NewLogger(serviceName string, development bool) (*zap.Logger, error) {
	var config zap.Config

	if development {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		config = zap.NewProductionConfig()
	}

	config.InitialFields = map[string]interface{}{
		"service": serviceName,
	}

	return config.Build()
}

// NewTestLogger 테스트용 로거 생성
func NewTestLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}
