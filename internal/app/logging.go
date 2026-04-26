package app

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

const levelCrit = slog.Level(12)

type multiHandler struct {
	handlers []slog.Handler
}

type spacingHandler struct {
	base slog.Handler
	out  io.Writer
}

type colorizingLogWriter struct {
	base io.Writer
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

func (h spacingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h spacingHandler) Handle(ctx context.Context, record slog.Record) error {
	if err := h.base.Handle(ctx, record); err != nil {
		return err
	}
	if _, err := io.WriteString(h.out, "\n"); err != nil {
		return err
	}
	return nil
}

func (h spacingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return spacingHandler{
		base: h.base.WithAttrs(attrs),
		out:  h.out,
	}
}

func (h spacingHandler) WithGroup(name string) slog.Handler {
	return spacingHandler{
		base: h.base.WithGroup(name),
		out:  h.out,
	}
}

func newConsoleHandler(out io.Writer, options *slog.HandlerOptions) slog.Handler {
	colorizedOut := colorizingLogWriter{base: out}
	return spacingHandler{
		base: slog.NewTextHandler(colorizedOut, options),
		out:  out,
	}
}

func newFileHandler(out io.Writer, options *slog.HandlerOptions) slog.Handler {
	return spacingHandler{
		base: slog.NewTextHandler(out, options),
		out:  out,
	}
}

func (w colorizingLogWriter) Write(p []byte) (int, error) {
	text := string(p)
	text = strings.ReplaceAll(text, "level=ERROR", "level="+colorRed+"ERROR"+colorReset)
	text = strings.ReplaceAll(text, "level=CRIT", "level="+colorRed+"CRIT"+colorReset)
	return w.base.Write([]byte(text))
}

func buildLogger(levelName string, logFile string, silent bool) (*slog.Logger, io.Closer, error) {
	level := parseLogLevel(levelName)
	options := &slog.HandlerOptions{Level: level, ReplaceAttr: replaceLogAttrs}
	handlers := make([]slog.Handler, 0, 2)
	if !silent {
		handlers = append(handlers, newConsoleHandler(os.Stderr, options))
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
	handlers = append(handlers, newFileHandler(file, options))
	if len(handlers) == 1 {
		return slog.New(handlers[0]), file, nil
	}
	return slog.New(multiHandler{handlers: handlers}), file, nil
}

func replaceLogAttrs(_ []string, attr slog.Attr) slog.Attr {
	if attr.Key == slog.LevelKey {
		if level, ok := attr.Value.Any().(slog.Level); ok && level == levelCrit {
			attr.Value = slog.StringValue("CRIT")
		}
	}
	return attr
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
		return levelCrit
	case "off":
		return slog.Level(100)
	default:
		return slog.LevelInfo
	}
}
