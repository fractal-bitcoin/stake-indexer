package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Log = zap.NewNop()
)

func Init() error {
	enc := zap.NewProductionEncoderConfig()
	enc.EncodeTime = zapcore.RFC3339TimeEncoder

	var err error
	Log, err = zap.Config{
		Encoding:          "json",
		Level:             zap.NewAtomicLevelAt(zapcore.DebugLevel),
		EncoderConfig:     enc,
		DisableCaller:     true,
		DisableStacktrace: true,
		OutputPaths:       []string{"stdout"},
	}.Build()
	if err != nil {
		return fmt.Errorf("build logger failed: %w", err)
	}
	return nil
}

func SyncLog() {
	_ = Log.Sync()
}
