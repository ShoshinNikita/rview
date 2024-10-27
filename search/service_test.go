package search

import (
	"context"
	"fmt"
	"testing"

	"github.com/ShoshinNikita/rview/pkg/cache"
	"github.com/stretchr/testify/require"
)

func TestService_RefreshIndexes(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()

	rclone := &rcloneStub{
		GetAllFilesFn: func(context.Context) (dirs, files []string, err error) {
			files = []string{
				"hello world.go",
				"arts/games/1.jpeg",
			}
			return dirs, files, nil
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

	rclone.GetAllFilesFn = func(context.Context) (dirs, files []string, err error) {
		files = []string{
			"hello world.go",
			"qwerty.txt",
		}
		return dirs, files, nil
	}

	err = s.RefreshIndexes(ctx)
	r.NoError(err)

	dirs, files, err = s.Search(ctx, "games", 3, 5)
	r.NoError(err)
	r.Empty(dirs)
	r.Empty(files)
}

type rcloneStub struct {
	GetAllFilesFn func(context.Context) (dirs, files []string, err error)
}

func (s rcloneStub) GetAllFiles(ctx context.Context) (dirs, files []string, err error) {
	return s.GetAllFilesFn(ctx)
}

// ExampleSearch generates an output in Markdown format that is used in documentation for search.
//
//nolint:govet
func ExampleSearch() {
	assertNoError := func(err error) {
		if err != nil {
			panic(err)
		}
	}

	files := []string{
		"animals/cute cat.jpeg",
		"animals/cat jumps.mp4",
		"animals/caterpillar.png",
		"animals/Cat & Dog play.mkv",
		"dogmas/catalog.zip",
	}
	tests := []struct {
		search string
		desc   string
		dirs   []string
		files  []string
	}{
		{
			search: `caterpillar`,
			desc:   "search for filepaths that have the same prefixes as `caterpillar` (`cat`, `cate`, `cater`, ...)",
		},
		{
			search: `"caterpillar"`,
			desc:   "search for filepaths that have exactly `caterpillar`",
		},
		{
			search: `cat dog`,
			desc:   "search for filepaths that have the same prefixes as both `cat` and `dog`",
		},
		{
			search: `cat dog -"zip"`,
			desc:   "search for filepaths that have the same prefixes as both `cat` and `dog`, but don't have exactly `zip`",
		},
		{
			search: `-"dog" -"png" -"jumps"`,
			desc:   "search for filepaths that don't have exactly `dog`, `png` and `jumps`",
		},
		{
			search: `dog "/cat" -"mkv"`,
			desc:   "search for filepaths that have the same prefixes as `dog`, have exactly `/cat` and don't have exactly `mkv`",
		},
	}

	rclone := &rcloneStub{
		GetAllFilesFn: func(context.Context) (_, _ []string, err error) { return nil, files, nil },
	}
	s := NewService(rclone, cache.NewInMemoryCache())
	assertNoError(s.Start())
	defer func() {
		assertNoError(s.Shutdown(context.Background()))
	}()

	fmt.Print("**Files:**\n\n")
	for _, f := range files {
		fmt.Printf("- `%s`\n", f)
	}

	fmt.Print("\n**Search Requests:**\n\n")
	for _, tt := range tests {
		_, files, err := s.Search(context.Background(), tt.search, 10, 10)
		assertNoError(err)

		fmt.Printf("- `%s` - %s. Results:\n", tt.search, tt.desc)
		for _, h := range files {
			fmt.Printf("  - `%s`\n", h.Path)
		}
	}

	// Output:
	//
	// **Files:**
	//
	// - `animals/cute cat.jpeg`
	// - `animals/cat jumps.mp4`
	// - `animals/caterpillar.png`
	// - `animals/Cat & Dog play.mkv`
	// - `dogmas/catalog.zip`
	//
	// **Search Requests:**
	//
	// - `caterpillar` - search for filepaths that have the same prefixes as `caterpillar` (`cat`, `cate`, `cater`, ...). Results:
	//   - `animals/caterpillar.png`
	//   - `animals/Cat & Dog play.mkv`
	//   - `animals/cat jumps.mp4`
	//   - `animals/cute cat.jpeg`
	//   - `dogmas/catalog.zip`
	// - `"caterpillar"` - search for filepaths that have exactly `caterpillar`. Results:
	//   - `animals/caterpillar.png`
	// - `cat dog` - search for filepaths that have the same prefixes as both `cat` and `dog`. Results:
	//   - `animals/Cat & Dog play.mkv`
	//   - `dogmas/catalog.zip`
	// - `cat dog -"zip"` - search for filepaths that have the same prefixes as both `cat` and `dog`, but don't have exactly `zip`. Results:
	//   - `animals/Cat & Dog play.mkv`
	// - `-"dog" -"png" -"jumps"` - search for filepaths that don't have exactly `dog`, `png` and `jumps`. Results:
	//   - `animals/cute cat.jpeg`
	// - `dog "/cat" -"mkv"` - search for filepaths that have the same prefixes as `dog`, have exactly `/cat` and don't have exactly `mkv`. Results:
	//   - `dogmas/catalog.zip`
}
