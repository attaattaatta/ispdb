package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"ispdb/internal/app"
)

const version = "0.1.7-beta"

//go:embed internal/ascii/*.txt
var asciiFS embed.FS

func main() {
	binaryName := filepath.Base(os.Args[0])
	cfg, err := app.ParseConfig(binaryName, os.Args[1:])
	if err != nil {
		if cfg.LogLevel != "off" {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		os.Exit(2)
	}

	application, err := app.New(cfg, version, asciiFS, binaryName)
	if err != nil {
		if cfg.LogLevel != "off" {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		os.Exit(1)
	}

	if err := application.Run(); err != nil {
		if cfg.LogLevel != "off" {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		os.Exit(1)
	}
}
