package app

import "testing"

func TestIsRemoteRootUID(t *testing.T) {
	t.Parallel()

	if !isRemoteRootUID("0\n") {
		t.Fatalf("expected uid 0 to be treated as root")
	}
	if isRemoteRootUID("1000\n") {
		t.Fatalf("did not expect non-zero uid to be treated as root")
	}
	if isRemoteRootUID("") {
		t.Fatalf("did not expect empty output to be treated as root")
	}
}
