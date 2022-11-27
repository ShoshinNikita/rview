package testutil

import (
	"errors"
	"strings"
	"testing"
)

func IsError(t *testing.T, err, target error) {
	t.Helper()

	if !errors.Is(err, target) {
		t.Fatalf("expected %v, got %v", target, err)
	}
}

func NoError(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("expected no error, got: %s", err)
	}
}

func Contains(t *testing.T, s, substr string) {
	t.Helper()

	if !strings.Contains(s, substr) {
		t.Fatalf("%q doesn't contain %q", s, substr)
	}
}

func Equal[T comparable](t *testing.T, want, got T) {
	t.Helper()

	if want != got {
		t.Fatalf("expected: %+v, got: %+v", want, got)
	}
}

func ElementsMatch[T comparable](t *testing.T, want, got []T) {
	t.Helper()

	if len(want) != len(got) {
		t.Fatalf("different number of elements: %d != %d, want: %+v, got: %+v", len(want), len(got), want, got)
	}

	var c int
	for i := range want {
		for j := range got {
			if want[i] == got[j] {
				c++
				break
			}
		}
	}
	if c != len(want) {
		t.Fatalf("different elements, want: %+v, got: %+v", want, got)
	}
}
