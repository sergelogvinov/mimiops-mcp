// Package logger provides the shared zap-based structured logger used across
// the mimiops-mcp server.
package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Level is the severity threshold derived from the --log-level flag.
type Level string

// Supported log levels, matching --log-level flag values.
const (
	Debug Level = "debug"
	Info  Level = "info"
	Warn  Level = "warn"
	Error Level = "error"
)

// Format is the output encoding requested via --log-format.
type Format string

// Supported output formats.
const (
	Text Format = "text"
	JSON Format = "json"
)

// Options controls logger construction.
type Options struct {
	Level  Level
	Format Format
}

// New builds a zap logger writing to stderr with the configured level and
// encoder. Caller is captured so logs show the source line.
func New(o Options) (*zap.Logger, error) {
	if err := o.Level.Validate(); err != nil {
		return nil, err
	}
	if err := o.Format.Validate(); err != nil {
		return nil, err
	}

	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(o.Level.zapLevel())

	if o.Format == Text {
		cfg.Encoding = "console"
		cfg.EncoderConfig = zap.NewDevelopmentEncoderConfig()
	} else {
		cfg.Encoding = "json"
	}

	log, err := cfg.Build(zap.AddCaller())
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}

	return log, nil
}

// zapLevel maps the option to a zapcore level.
func (l Level) zapLevel() zapcore.Level {
	switch l {
	case Debug:
		return zapcore.DebugLevel
	case Warn:
		return zapcore.WarnLevel
	case Error:
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// Validate reports whether the level is a known value.
func (l Level) Validate() error {
	switch l {
	case Debug, Info, Warn, Error:
		return nil
	default:
		return fmt.Errorf("invalid log level %q: must be one of debug, info, warn, error", l)
	}
}

// Validate reports whether the format is a known value.
func (f Format) Validate() error {
	switch f {
	case Text, JSON:
		return nil
	default:
		return fmt.Errorf("invalid log format %q: must be one of text, json", f)
	}
}
