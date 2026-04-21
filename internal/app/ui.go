package app

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"strings"
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
}

func (u *UI) Info(text string) {
	if u.silent {
		return
	}
	fmt.Fprintln(u.out, text)
}

func (u *UI) Success(text string) {
	if u.silent {
		return
	}
	fmt.Fprintln(u.out, colorizeStatusText(text, "OK", colorGreen))
}

func (u *UI) Warn(text string) {
	if u.silent {
		return
	}
	fmt.Fprintln(u.out, colorizeColonSuffix(text, colorYellow))
}

func (u *UI) Error(text string) {
	if u.silent {
		return
	}
	fmt.Fprintln(u.err, colorizeColonSuffix(text, colorRed))
}

func (u *UI) PrintASCII(arts []string) {
	if u.silent {
		return
	}
	if len(arts) == 0 {
		return
	}
	fmt.Fprintln(u.out, arts[u.rng.Intn(len(arts))])
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
