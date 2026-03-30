package logger

import (
	"fmt"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	*zap.Logger
	atomicLevel zap.AtomicLevel
}

type Config struct {
	Level             string                // "debug" | "info" | "warn" | "error" | ...
	JSON              bool                  // if true -> JSON encoder
	OutputFile        string                // optional file path
	SuppressStdout    bool                  // if true, skip writing to os.Stdout (use when logging to file only)
	ExtraWriteSyncers []zapcore.WriteSyncer // optional extra sinks
}

func New(cfg Config) (*Logger, error) {
	// level (runtime changeable)
	atomic := zap.NewAtomicLevelAt(zapcore.InfoLevel)
	if cfg.Level != "" {
		var lvl zapcore.Level
		if err := lvl.UnmarshalText([]byte(cfg.Level)); err == nil {
			atomic.SetLevel(lvl)
		}
	}

	// encoder config
	encCfg := zap.NewProductionEncoderConfig()
	encCfg.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006/01/02 15:04:05"))
	}

	// Only color in console mode. JSON must NOT contain ANSI colors.
	if cfg.JSON {
		encCfg.EncodeLevel = zapcore.LowercaseLevelEncoder
	} else {
		encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	encCfg.CallerKey = "caller"
	encCfg.EncodeCaller = zapcore.ShortCallerEncoder

	var enc zapcore.Encoder
	if cfg.JSON {
		enc = zapcore.NewJSONEncoder(encCfg)
	} else {
		enc = zapcore.NewConsoleEncoder(encCfg)
	}

	// sinks
	var ws []zapcore.WriteSyncer
	if !cfg.SuppressStdout {
		ws = append(ws, zapcore.AddSync(os.Stdout))
	}
	for _, extra := range cfg.ExtraWriteSyncers {
		ws = append(ws, extra)
	}

	if cfg.OutputFile != "" {
		f, err := os.OpenFile(cfg.OutputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		ws = append(ws, zapcore.AddSync(f))
	}

	core := zapcore.NewCore(enc, zapcore.NewMultiWriteSyncer(ws...), atomic)
	z := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	return &Logger{
		Logger:      z,
		atomicLevel: atomic,
	}, nil
}

func (l *Logger) Sync() error {
	return l.Logger.Sync()
}

func (l *Logger) SetLevel(level zapcore.Level) {
	l.atomicLevel.SetLevel(level)
}

func MinimalLoggerConfig() middleware.RequestLoggerConfig {
	return middleware.RequestLoggerConfig{
		LogURI:     true,
		LogStatus:  true,
		LogMethod:  true,
		LogLatency: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			fmt.Fprintf(os.Stdout,
				"%s INFO REQUEST method=%s uri=%s status=%d latency=%s\n",
				time.Now().Format("2006/01/02 15:04:05"),
				v.Method,
				v.URI,
				v.Status,
				v.Latency.String(),
			)
			return nil
		},
	}

}
