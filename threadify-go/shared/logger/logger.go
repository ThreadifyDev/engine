package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewLogger(production bool) (*zap.Logger, error) {
	var config zap.Config

	if production {
		config = zap.NewProductionConfig()
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		config.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	} else {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	if levelStr := os.Getenv("LOG_LEVEL"); levelStr != "" {
		var level zapcore.Level
		if err := level.UnmarshalText([]byte(levelStr)); err == nil {
			config.Level = zap.NewAtomicLevelAt(level)
		}
	}

	if config.OutputPaths == nil {
		config.OutputPaths = []string{"stdout"}
	}
	if config.ErrorOutputPaths == nil {
		config.ErrorOutputPaths = []string{"stderr"}
	}

	return config.Build()
}
