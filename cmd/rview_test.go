package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/ShoshinNikita/rview/pkg/util/testutil"
)

func TestSafeShutdown(t *testing.T) {
	ctx := context.Background()

	err := safeShutdown(ctx, nil)
	testutil.NoError(t, err)

	err = safeShutdown(ctx, (*testShutdowner)(nil))
	testutil.NoError(t, err)

	err = safeShutdown(ctx, new(testShutdowner))
	testutil.Equal(t, err.Error(), "test")
}

type testShutdowner struct{}

func (*testShutdowner) Shutdown(context.Context) error { return errors.New("test") }
