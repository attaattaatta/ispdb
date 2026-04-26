package app

import (
	"reflect"
	"testing"
)

func TestDestExecutionScopesFromValueDefaultsToCanonicalOrder(t *testing.T) {
	t.Parallel()

	got := destExecutionScopesFromValue("all")
	want := []string{"packages", "users", "dns", "webdomains", "databases", "email"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("destExecutionScopesFromValue(all) = %#v, want %#v", got, want)
	}
}

func TestDestExecutionScopesFromValueNormalizesSubsetOrder(t *testing.T) {
	t.Parallel()

	got := destExecutionScopesFromValue("email,packages,dns,users")
	want := []string{"packages", "users", "dns", "email"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("destExecutionScopesFromValue(email,packages,dns,users) = %#v, want %#v", got, want)
	}
}

func TestDestExecutionScopesFromValueLeavesSingleScopeUntouched(t *testing.T) {
	t.Parallel()

	got := destExecutionScopesFromValue("users")
	want := []string{"users"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("destExecutionScopesFromValue(users) = %#v, want %#v", got, want)
	}
}

func TestDestExecutionScopesFromValueKeepsEmailWithoutPackages(t *testing.T) {
	t.Parallel()

	got := destExecutionScopesFromValue("email")
	want := []string{"email"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("destExecutionScopesFromValue(email) = %#v, want %#v", got, want)
	}
}
