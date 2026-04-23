package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"ispdb/internal/app"
)

const version = "0.4.2-beta"

//go:embed internal/ascii/*.txt
var asciiFS embed.FS

func main() {
	stopInterruptHandler := app.SetupInterruptHandler()
	defer stopInterruptHandler()

	binaryName := filepath.Base(os.Args[0])
	if app.RequiresRootForArgs(os.Args[1:]) {
		if err := app.CheckRootPreflight(); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, helpTip())
			os.Exit(1)
		}
	}
	cfg, err := app.ParseConfig(binaryName, os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		if cfg.LogLevel != "off" {
			if tip := parseErrorTip(err.Error()); tip != "" {
				fmt.Fprintln(os.Stderr)
				fmt.Fprintln(os.Stderr, tip)
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

func parseErrorTip(message string) string {
	if message == "" {
		return ""
	}
	return helpTip()
}

func helpTip() string {
	return "Tip: -h, --help to show help"
}
