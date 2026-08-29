package require

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type Require struct {
	t testing.TB
}

func New(t testing.TB) *Require {
	return &Require{t}
}

func (r *Require) Error(err error) {
	r.t.Helper()

	if err == nil {
		r.t.Fatalf("unexpected <nil> error")
	}
}

func (r *Require) NoError(err error) {
	r.t.Helper()

	if err != nil {
		r.t.Fatalf("unexpected error: %s", err)
	}
}

func (r *Require) Equal[T any](a, b T) {
	r.t.Helper()

	diff := cmp.Diff(a, b, cmp.AllowUnexported())
	if diff != "" {
		r.t.Fatalf("values are not equal (-a +b):\n%s", diff)
	}
}

func (r *Require) NotEqual[T any](a, b T) {
	r.t.Helper()

	if cmp.Equal(a, b) {
		r.t.Fatalf("values are equal: %+v", a)
	}
}

func (r *Require) True(v bool) {
	r.t.Helper()

	if !v {
		r.t.Fatalf("unexpected false")
	}
}

func (r *Require) False(v bool) {
	r.t.Helper()

	if v {
		r.t.Fatalf("unexpected true")
	}
}

func (r *Require) Len(v any, length int) {
	r.t.Helper()

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Chan:
		if rv.Len() != length {
			r.t.Fatalf("expected %d elements, got %d: %+v", length, rv.Len(), v)
		}
	default:
		r.t.Fatalf("unsupported type: %T", v)
	}
}

func (r *Require) Contains[A, B string | []byte](s A, substr B) {
	r.t.Helper()

	if !strings.Contains(string(s), string(substr)) {
		r.t.Fatalf("%q doesn't contains %q", s, substr)
	}
}
