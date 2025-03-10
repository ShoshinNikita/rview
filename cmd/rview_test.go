package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeShutdown(t *testing.T) {
	r := require.New(t)
	ctx := t.Context()

	err := safeShutdown(ctx, nil)
	r.NoError(err)

	err = safeShutdown(ctx, (*testShutdowner)(nil))
	r.NoError(err)

	err = safeShutdown(ctx, new(testShutdowner))
	r.EqualError(err, "test")
}

type testShutdowner struct{}

func (*testShutdowner) Shutdown(context.Context) error { return errors.New("test") }
