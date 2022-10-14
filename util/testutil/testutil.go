package testutil

import "testing"

func Equal[T comparable](t *testing.T, want, got T) {
	t.Helper()

	if want != got {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
