package app

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

func askYesNo(question string, defaultNo bool) (bool, error) {
	return askYesNoWithColor(question, defaultNo, colorGreen)
}

func askYesNoWithColor(question string, defaultNo bool, color string) (bool, error) {
	prompt := "[y/N]"
	if !defaultNo {
		prompt = "[Y/n]"
	}
	fmt.Printf("%s%s %s%s ", color, question, prompt, colorReset)

	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			return false, err
		}
		defer term.Restore(fd, oldState)

		buffer := make([]byte, 0, 4)
		byteBuf := make([]byte, 1)
		for {
			if _, err := os.Stdin.Read(byteBuf); err != nil {
				return false, err
			}
			if byteBuf[0] == 3 {
				os.Exit(130)
			}
			buffer = append(buffer, byteBuf[0])
			if !utf8.FullRune(buffer) {
				continue
			}
			r, _ := utf8.DecodeRune(buffer)
			buffer = buffer[:0]
			switch r {
			case 'y', 'Y', '\u043d', '\u041d':
				fmt.Print("\r\n")
				return true, nil
			case 'n', 'N', '\u0442', '\u0422':
				fmt.Print("\r\n")
				return false, nil
			}
		}
	}

	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	if text == "" {
		return !defaultNo, nil
	}
	r, _ := utf8.DecodeRuneInString(text)
	switch r {
	case 'y', 'Y', '\u043d', '\u041d':
		return true, nil
	case 'n', 'N', '\u0442', '\u0422':
		return false, nil
	default:
		return !defaultNo, nil
	}
}

func askSecret(question string) (string, error) {
	fmt.Printf("%s%s%s ", colorGreen, question, colorReset)
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		value, err := term.ReadPassword(fd)
		fmt.Println("")
		return string(value), err
	}
	reader := bufio.NewReader(os.Stdin)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func askLines(question string) ([]string, error) {
	fmt.Printf("%s%s%s\n", colorGreen, question, colorReset)
	fmt.Println("Submit an empty line to finish input.")

	reader := bufio.NewReader(os.Stdin)
	lines := make([]string, 0)
	for {
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		lines = append(lines, line)
		if err != nil {
			break
		}
	}
	return lines, nil
}
