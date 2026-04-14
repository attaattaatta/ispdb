package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ispdb/internal/app"
)

const version = "0.2.1-beta"

//go:embed internal/ascii/*.txt
var asciiFS embed.FS

func main() {
	binaryName := filepath.Base(os.Args[0])
	cfg, err := app.ParseConfig(binaryName, os.Args[1:])
	if err != nil {
		if cfg.LogLevel != "off" {
			fmt.Fprintln(os.Stderr, err.Error())
			if shouldShowHelpAfterParseError(err.Error()) {
				fmt.Fprintln(os.Stderr)
				fmt.Fprintln(os.Stderr, app.HelpText(version, binaryName))
			}
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

func shouldShowHelpAfterParseError(message string) bool {
	hints := []string{
		"Supported values:",
		"Tip:",
		"requires --",
		"can be used only",
	}
	for _, hint := range hints {
		if strings.Contains(message, hint) {
			return false
		}
	}
	return true
}
