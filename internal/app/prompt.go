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
	promptText := fmt.Sprintf("%s%s %s%s ", color, question, prompt, colorReset)
	fmt.Print(promptText)
	mirrorProgramOutput(promptText)

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
				handleInterruptRequest()
				buffer = buffer[:0]
				continue
			}
			if byteBuf[0] == '\r' || byteBuf[0] == '\n' {
				fmt.Print("\r\n")
				if defaultNo {
					mirrorProgramOutput("n\n")
				} else {
					mirrorProgramOutput("y\n")
				}
				return !defaultNo, nil
			}
			buffer = append(buffer, byteBuf[0])
			if !utf8.FullRune(buffer) {
				continue
			}
			r, _ := utf8.DecodeRune(buffer)
			buffer = buffer[:0]
			switch interpretYesNoRune(r, defaultNo) {
			case yesNoAnswerYes:
				fmt.Print("\r\n")
				mirrorProgramOutput("y\n")
				return true, nil
			case yesNoAnswerNo:
				fmt.Print("\r\n")
				mirrorProgramOutput("n\n")
				return false, nil
			}
		}
	}

	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	mirrorProgramOutput(text)
	if text == "" {
		return !defaultNo, nil
	}
	r, _ := utf8.DecodeRuneInString(text)
	switch interpretYesNoRune(r, defaultNo) {
	case yesNoAnswerYes:
		return true, nil
	case yesNoAnswerNo:
		return false, nil
	default:
		return !defaultNo, nil
	}
}

type yesNoAnswer int

const (
	yesNoAnswerUnknown yesNoAnswer = iota
	yesNoAnswerYes
	yesNoAnswerNo
)

func interpretYesNoRune(r rune, defaultNo bool) yesNoAnswer {
	switch r {
	case 'y', 'Y', '\u043d', '\u041d':
		return yesNoAnswerYes
	case 'n', 'N', '\u0442', '\u0422':
		return yesNoAnswerNo
	case '\r', '\n':
		if defaultNo {
			return yesNoAnswerNo
		}
		return yesNoAnswerYes
	default:
		return yesNoAnswerUnknown
	}
}

func askSecret(question string) (string, error) {
	promptText := fmt.Sprintf("%s%s%s ", colorGreen, question, colorReset)
	fmt.Print(promptText)
	mirrorProgramOutput(promptText)
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		value, err := term.ReadPassword(fd)
		fmt.Println("")
		mirrorProgramOutput("<hidden>\n")
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
	promptText := fmt.Sprintf("%s%s%s\n", colorGreen, question, colorReset)
	fmt.Print(promptText)
	mirrorProgramOutput(promptText)
	fmt.Println("Submit an empty line to finish input.")
	mirrorProgramOutput("Submit an empty line to finish input.\n")

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
		mirrorProgramOutput(line + "\n")
		lines = append(lines, line)
		if err != nil {
			break
		}
	}
	return lines, nil
}
