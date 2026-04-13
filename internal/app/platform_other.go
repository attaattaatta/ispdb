//go:build !linux

package app

import (
	"fmt"
	"os"
)

func ensureLinux() error {
	return fmt.Errorf("%sthis application supports Linux only%s", colorRed, colorReset)
}

func ensureRoot() error {
	return nil
}

func acquireLock(string) (*os.File, error) {
	return nil, nil
}
