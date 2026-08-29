package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/ShoshinNikita/rview/pkg/require"
)

func TestSafeShutdown(t *testing.T) {
	r := require.New(t)
	ctx := t.Context()

	err := safeShutdown(ctx, nil)
	r.NoError(err)

	err = safeShutdown(ctx, (*testShutdowner)(nil))
	r.NoError(err)

	err = safeShutdown(ctx, new(testShutdowner))
	r.Error(err)
	r.Equal(err.Error(), "test")
}

type testShutdowner struct{}

func (*testShutdowner) Shutdown(context.Context) error { return errors.New("test") }
