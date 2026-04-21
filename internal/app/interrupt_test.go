package app

import (
	"bytes"
	"testing"
)

func TestHandleInterruptRequestWarnsOnFirstPress(t *testing.T) {
	originalOut := interruptOut
	originalExit := interruptExitHook
	interruptCount.Store(0)
	t.Cleanup(func() {
		interruptOut = originalOut
		interruptExitHook = originalExit
		interruptCount.Store(0)
	})

	var out bytes.Buffer
	exitCalled := false
	interruptOut = &out
	interruptExitHook = func(code int) {
		exitCalled = true
	}

	handleInterruptRequest()

	if exitCalled {
		t.Fatalf("did not expect process exit on first Ctrl+C")
	}
	got := out.String()
	want := "\nPress Ctrl+C again to terminate program.\n"
	if got != want {
		t.Fatalf("first interrupt output = %q, want %q", got, want)
	}
}

func TestHandleInterruptRequestExitsOnSecondPress(t *testing.T) {
	originalOut := interruptOut
	originalExit := interruptExitHook
	interruptCount.Store(0)
	t.Cleanup(func() {
		interruptOut = originalOut
		interruptExitHook = originalExit
		interruptCount.Store(0)
	})

	var out bytes.Buffer
	exitCode := -1
	interruptOut = &out
	interruptExitHook = func(code int) {
		exitCode = code
	}

	handleInterruptRequest()
	handleInterruptRequest()

	if exitCode != 130 {
		t.Fatalf("expected second Ctrl+C to exit with code 130, got %d", exitCode)
	}
}
