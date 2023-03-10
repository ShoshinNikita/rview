package search

import (
	"context"
	"testing"

	"github.com/ShoshinNikita/rview/pkg/cache"
	"github.com/stretchr/testify/require"
)

// Other methods are checked by integration tests.

func TestService_RefreshIndexes(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()

	rclone := &rcloneStub{
		GetAllFilesFn: func(context.Context) ([]string, error) {
			return []string{
				"hello world.go",
				"arts/games/1.jpeg",
			}, nil
		},
	}
	s := NewService(rclone, cache.NewInMemoryCache())
	err := s.Start()
	r.NoError(err)
	t.Cleanup(func() {
		err := s.Shutdown(ctx)
		r.NoError(err)
	})

	dirs, files, err := s.Search(ctx, "games", 3, 5)
	r.NoError(err)
	r.Empty(dirs)
	r.NotEmpty(files)

	rclone.GetAllFilesFn = func(context.Context) ([]string, error) {
		return []string{
			"hello world.go",
			"qwerty.txt",
		}, nil
	}

	err = s.RefreshIndexes(ctx)
	r.NoError(err)

	dirs, files, err = s.Search(ctx, "games", 3, 5)
	r.NoError(err)
	r.Empty(dirs)
	r.Empty(files)
}

type rcloneStub struct {
	GetAllFilesFn func(context.Context) ([]string, error)
}

func (s rcloneStub) GetAllFiles(ctx context.Context) ([]string, error) {
	return s.GetAllFilesFn(ctx)
}
