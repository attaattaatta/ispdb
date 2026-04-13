//go:build linux

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func ensureLinux() error {
	return nil
}

func ensureRoot() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("%sroot privileges are required%s", colorRed, colorReset)
	}
	return nil
}

func acquireLock(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("%sanother ispdb process is already running%s", colorRed, colorReset)
	}
	return file, nil
}
