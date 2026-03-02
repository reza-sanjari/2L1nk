package logger

import (
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	*zap.Logger
	atomicLevel zap.AtomicLevel
}

type Config struct {
	Level      string // "debug" | "info" | "warn" | "error" | ...
	JSON       bool   // if true -> JSON encoder
	OutputFile string // optional
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
	ws := []zapcore.WriteSyncer{zapcore.AddSync(os.Stdout)}

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
