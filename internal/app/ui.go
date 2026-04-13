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
	fmt.Fprintln(u.out, highlightColon(text))
}

func (u *UI) Success(text string) {
	if u.silent {
		return
	}
	fmt.Fprintf(u.out, "%s%s%s\n", colorGreen, text, colorReset)
}

func (u *UI) Warn(text string) {
	if u.silent {
		return
	}
	fmt.Fprintf(u.out, "%s%s%s\n", colorYellow, text, colorReset)
}

func (u *UI) Error(text string) {
	if u.silent {
		return
	}
	fmt.Fprintf(u.err, "%s%s%s\n", colorRed, text, colorReset)
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

func highlightColon(text string) string {
	index := strings.Index(text, ":")
	if index == -1 {
		return colorGreen + text + colorReset
	}
	return colorGreen + text[:index+1] + colorReset + text[index+1:]
}
