package app

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestConsoleHandlerAddsBlankLineAfterInfo(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	handler := newConsoleHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})

	record := slog.NewRecord(testTime(), slog.LevelInfo, "remote command was adjusted to destination form", 0)
	record.AddAttrs(slog.String("note", "removed before execution"))
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	got := out.String()
	if !strings.HasSuffix(got, "\n\n") {
		t.Fatalf("expected info log to end with an extra blank line, got %q", got)
	}
}

func TestConsoleHandlerAddsBlankLineAfterWarn(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	handler := newConsoleHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})

	record := slog.NewRecord(testTime(), slog.LevelWarn, "warning", 0)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	got := out.String()
	if !strings.HasSuffix(got, "\n\n") {
		t.Fatalf("expected warn log to end with an extra blank line, got %q", got)
	}
}

func TestConsoleHandlerColorsErrorLevel(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	handler := newConsoleHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})

	record := slog.NewRecord(testTime(), slog.LevelError, "ssh command failed", 0)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	got := out.String()
	want := "level=" + colorRed + "ERROR" + colorReset
	if !strings.Contains(got, want) {
		t.Fatalf("expected colored error level, got %q", got)
	}
}

func TestConsoleHandlerColorsCritLevel(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	handler := newConsoleHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug, ReplaceAttr: replaceLogAttrs})

	record := slog.NewRecord(testTime(), levelCrit, "package step timed out", 0)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	got := out.String()
	want := "level=" + colorRed + "CRIT" + colorReset
	if !strings.Contains(got, want) {
		t.Fatalf("expected colored crit level, got %q", got)
	}
}

func TestFileHandlerAddsBlankLineAfterInfo(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	handler := newFileHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})

	record := slog.NewRecord(testTime(), slog.LevelInfo, "building source data model", 0)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	got := out.String()
	if !strings.HasSuffix(got, "\n\n") {
		t.Fatalf("expected file log to end with an extra blank line, got %q", got)
	}
}

func TestParseLogLevelOffSuppressesOutput(t *testing.T) {
	t.Parallel()

	level := parseLogLevel("off")
	if level <= slog.LevelError {
		t.Fatalf("expected off level to be above normal log levels, got %v", level)
	}
}

func testTime() time.Time {
	return time.Date(2026, time.April, 20, 0, 0, 0, 0, time.UTC)
}
