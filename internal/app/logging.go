package app

import (
	"context"
	"io"
	"log/slog"
	"os"
)

type multiHandler struct {
	handlers []slog.Handler
}

func (m multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range m.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m multiHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, handler := range m.handlers {
		if handler.Enabled(ctx, record.Level) {
			if err := handler.Handle(ctx, record); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, 0, len(m.handlers))
	for _, handler := range m.handlers {
		handlers = append(handlers, handler.WithAttrs(attrs))
	}
	return multiHandler{handlers: handlers}
}

func (m multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, 0, len(m.handlers))
	for _, handler := range m.handlers {
		handlers = append(handlers, handler.WithGroup(name))
	}
	return multiHandler{handlers: handlers}
}

func buildLogger(levelName string, logFile string, silent bool) (*slog.Logger, io.Closer, error) {
	level := parseLogLevel(levelName)
	options := &slog.HandlerOptions{Level: level}
	handlers := make([]slog.Handler, 0, 2)
	if !silent {
		handlers = append(handlers, slog.NewTextHandler(os.Stderr, options))
	}
	if logFile == "" {
		if len(handlers) == 0 {
			return slog.New(slog.NewTextHandler(io.Discard, options)), nil, nil
		}
		if len(handlers) == 1 {
			return slog.New(handlers[0]), nil, nil
		}
		return slog.New(multiHandler{handlers: handlers}), nil, nil
	}

	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, err
	}
	handlers = append(handlers, slog.NewTextHandler(file, options))
	if len(handlers) == 1 {
		return slog.New(handlers[0]), file, nil
	}
	return slog.New(multiHandler{handlers: handlers}), file, nil
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "crit":
		return slog.Level(12)
	case "off":
		return slog.LevelInfo
	default:
		return slog.LevelInfo
	}
}
