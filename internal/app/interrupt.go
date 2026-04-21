package app

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync/atomic"
)

var (
	interruptCount    atomic.Int32
	interruptOut      io.Writer = os.Stderr
	interruptExitHook           = func(code int) { os.Exit(code) }
)

func SetupInterruptHandler() func() {
	interruptCount.Store(0)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	stopped := make(chan struct{})

	go func() {
		for {
			select {
			case <-signals:
				handleInterruptRequest()
			case <-stopped:
				signal.Stop(signals)
				return
			}
		}
	}()

	return func() {
		close(stopped)
		interruptCount.Store(0)
	}
}

func handleInterruptRequest() {
	if interruptCount.Add(1) == 1 {
		fmt.Fprintln(interruptOut)
		fmt.Fprintln(interruptOut, "Press Ctrl+C again to terminate program.")
		return
	}
	interruptExitHook(130)
}
