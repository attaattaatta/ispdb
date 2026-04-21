package app

import "testing"

func TestInterpretYesNoRuneSupportsDefaultOnEnter(t *testing.T) {
	t.Parallel()

	if got := interpretYesNoRune('\n', true); got != yesNoAnswerNo {
		t.Fatalf("interpretYesNoRune(newline, defaultNo=true) = %v, want %v", got, yesNoAnswerNo)
	}
	if got := interpretYesNoRune('\r', false); got != yesNoAnswerYes {
		t.Fatalf("interpretYesNoRune(carriage return, defaultNo=false) = %v, want %v", got, yesNoAnswerYes)
	}
}

func TestInterpretYesNoRuneSupportsRussianLayout(t *testing.T) {
	t.Parallel()

	if got := interpretYesNoRune('\u043d', true); got != yesNoAnswerYes {
		t.Fatalf("interpretYesNoRune(н) = %v, want %v", got, yesNoAnswerYes)
	}
	if got := interpretYesNoRune('\u0422', true); got != yesNoAnswerNo {
		t.Fatalf("interpretYesNoRune(Т) = %v, want %v", got, yesNoAnswerNo)
	}
}
