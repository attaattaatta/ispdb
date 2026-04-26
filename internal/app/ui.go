package app

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[0;91m"
	colorGreen  = "\033[0;92m"
	colorYellow = "\033[1;33m"
)

type UI struct {
	out    io.Writer
	err    io.Writer
	rng    *rand.Rand
	silent bool
}

var programOutputLog = struct {
	sync.Mutex
	file string
}{}

func NewUI() *UI {
	return &UI{
		out: os.Stdout,
		err: os.Stderr,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (u *UI) Println(text string) {
	if u.silent {
		return
	}
	fmt.Fprintln(u.out, text)
	mirrorProgramOutput(text + "\n")
}

func (u *UI) Info(text string) {
	if u.silent {
		return
	}
	fmt.Fprintln(u.out, text)
	mirrorProgramOutput(text + "\n")
}

func (u *UI) Success(text string) {
	if u.silent {
		return
	}
	line := colorizeStatusText(text, "OK", colorGreen)
	fmt.Fprintln(u.out, line)
	mirrorProgramOutput(line + "\n")
}

func (u *UI) Warn(text string) {
	if u.silent {
		return
	}
	line := colorizeColonSuffix(text, colorYellow)
	fmt.Fprintln(u.out, line)
	mirrorProgramOutput(line + "\n")
}

func (u *UI) Error(text string) {
	if u.silent {
		return
	}
	line := colorizeColonSuffix(text, colorRed)
	fmt.Fprintln(u.err, line)
	mirrorProgramOutput(line + "\n")
}

func (u *UI) PrintASCII(arts []string) {
	if u.silent {
		return
	}
	if len(arts) == 0 {
		return
	}
	text := arts[u.rng.Intn(len(arts))]
	fmt.Fprintln(u.out, text)
	mirrorProgramOutput(text + "\n")
}

func setProgramOutputLogFile(path string) {
	programOutputLog.Lock()
	defer programOutputLog.Unlock()
	programOutputLog.file = strings.TrimSpace(path)
}

func programOutputLogFile() string {
	programOutputLog.Lock()
	defer programOutputLog.Unlock()
	return programOutputLog.file
}

func mirrorProgramOutput(text string) {
	if text == "" {
		return
	}
	path := programOutputLogFile()
	if path == "" {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.WriteString(stripANSIEscapes(text))
}

func MirrorProgramOutput(text string) {
	mirrorProgramOutput(text)
}

func stripANSIEscapes(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for index := 0; index < len(value); {
		if value[index] == 0x1b && index+1 < len(value) && value[index+1] == '[' {
			index += 2
			for index < len(value) {
				ch := value[index]
				index++
				if ch >= '@' && ch <= '~' {
					break
				}
			}
			continue
		}
		builder.WriteByte(value[index])
		index++
	}
	return builder.String()
}

func colorizeStatusText(text string, token string, color string) string {
	suffix := ": " + token
	if strings.HasSuffix(text, suffix) {
		return strings.TrimSuffix(text, token) + color + token + colorReset
	}
	return colorizeColonSuffix(text, color)
}

func colorizeColonSuffix(text string, color string) string {
	index := strings.LastIndex(text, ":")
	if index < 0 || index+1 >= len(text) {
		return text
	}
	prefix := text[:index+1]
	suffix := strings.TrimLeft(text[index+1:], " ")
	if suffix == "" {
		return text
	}
	return prefix + " " + color + suffix + colorReset
}
