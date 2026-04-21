package app

import (
	"reflect"
	"testing"
)

func TestConsumeLogTextLinesKeepsPartialLineBetweenChunks(t *testing.T) {
	t.Parallel()

	var lines []string
	remainder, err := consumeLogTextLines("", "first line\nsecond", false, func(line string) error {
		lines = append(lines, line)
		return nil
	})
	if err != nil {
		t.Fatalf("consumeLogTextLines() returned error: %v", err)
	}
	if !reflect.DeepEqual(lines, []string{"first line"}) {
		t.Fatalf("unexpected lines after first chunk: %#v", lines)
	}
	if remainder != "second" {
		t.Fatalf("unexpected remainder after first chunk: %q", remainder)
	}

	remainder, err = consumeLogTextLines(remainder, " line\nthird line", true, func(line string) error {
		lines = append(lines, line)
		return nil
	})
	if err != nil {
		t.Fatalf("consumeLogTextLines() returned error: %v", err)
	}
	if remainder != "" {
		t.Fatalf("expected empty remainder after flush, got %q", remainder)
	}
	want := []string{"first line", "second line", "third line"}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("unexpected lines after flush: got %#v, want %#v", lines, want)
	}
}

func TestConsumeLogTextLinesTrimsEmptyLines(t *testing.T) {
	t.Parallel()

	var lines []string
	_, err := consumeLogTextLines("", "\n alpha \n\n beta \n", true, func(line string) error {
		lines = append(lines, line)
		return nil
	})
	if err != nil {
		t.Fatalf("consumeLogTextLines() returned error: %v", err)
	}

	want := []string{"alpha", "beta"}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("unexpected trimmed lines: got %#v, want %#v", lines, want)
	}
}
