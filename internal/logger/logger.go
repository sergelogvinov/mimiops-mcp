// Package logger provides the shared slog-based structured logger used across
// the mimiops-mcp server.
package logger

import (
	"fmt"
	"log/slog"
	"os"
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

// New builds a slog logger writing to stderr with the configured level and
// encoder. Caller is captured so logs show the source line.
func New(o Options) (*slog.Logger, error) {
	if err := o.Level.Validate(); err != nil {
		return nil, err
	}
	if err := o.Format.Validate(); err != nil {
		return nil, err
	}

	opts := &slog.HandlerOptions{
		Level: o.Level.slogLevel(),
	}

	// All hook output goes to stderr. In stdio mode, stdout is the JSON-RPC transport,
	// writing to stdout from hooks corrupts the protocol stream.
	var handler slog.Handler
	if o.Format == Text {
		handler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}

	return slog.New(handler), nil
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

// slogLevel maps the option to a slog.Level.
func (l Level) slogLevel() slog.Level {
	switch l {
	case Debug:
		return slog.LevelDebug
	case Warn:
		return slog.LevelWarn
	case Error:
		return slog.LevelError
	case Info:
		return slog.LevelInfo
	default:
		return slog.LevelInfo
	}
}
