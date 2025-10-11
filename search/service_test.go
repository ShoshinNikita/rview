package search

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/ShoshinNikita/rview/rclone"
	"github.com/stretchr/testify/require"
)

func TestService_RefreshIndex(t *testing.T) {
	r := require.New(t)
	ctx := t.Context()

	root, err := os.OpenRoot(t.TempDir())
	r.NoError(err)

	rcloneStub := &rcloneStub{
		GetAllFilesFn: func(context.Context) ([]rclone.DirEntry, error) {
			return []rclone.DirEntry{
				newDirEntry("/hello world.go", 0, 0),
				newDirEntry("/gaming.txt", 0, 0),
				newDirEntry("/arts/", 0, 0),
				newDirEntry("/arts/games/", 0, 0),
				newDirEntry("/arts/games/1.jpeg", 0, 0),
			}, nil
		},
	}
	s, err := NewService(rcloneStub, root)
	r.NoError(err)
	err = s.Start()
	r.NoError(err)
	defer func() {
		err := s.Shutdown(t.Context())
		r.NoError(err)
	}()

	hits, _, err := s.Search(ctx, "games", 5)
	r.NoError(err)
	r.Equal(
		[]Hit{
			{Path: "/arts/games/", IsDir: true, Score: 3},
			{Path: "/gaming.txt", IsDir: false, Score: 1},
		},
		hits,
	)

	rcloneStub.GetAllFilesFn = func(context.Context) ([]rclone.DirEntry, error) {
		return []rclone.DirEntry{
			newDirEntry("/hello world.go", 0, 0),
			newDirEntry("/qwerty.txt", 0, 0),
		}, nil
	}

	err = s.RefreshIndex(ctx)
	r.NoError(err)

	hits, _, err = s.Search(ctx, "games", 5)
	r.NoError(err)
	r.Empty(hits)
}

type rcloneStub struct {
	GetAllFilesFn func(context.Context) ([]rclone.DirEntry, error)
}

func (s rcloneStub) GetAllFiles(ctx context.Context) ([]rclone.DirEntry, error) {
	return s.GetAllFilesFn(ctx)
}

// TestService_GenerateDocs generates an output in Markdown format that is used in documentation for search.
func TestGenerateDocs(t *testing.T) {
	r := require.New(t)

	root, err := os.OpenRoot(t.TempDir())
	r.NoError(err)

	entries := []rclone.DirEntry{
		newDirEntry("/animals/cute cat.jpeg", 0, 0),
		newDirEntry("/animals/cat jumps.mp4", 0, 0),
		newDirEntry("/animals/caterpillar.png", 0, 0),
		newDirEntry("/animals/Cat & Dog play.mkv", 0, 0),
		newDirEntry("/dogmas/catalog.zip", 0, 0),
	}
	tests := []struct {
		search string
		desc   string
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
			search: `cat dog -zip`,
			desc:   "search for filepaths that have the same prefixes as both `cat` and `dog`, but don't have exactly `zip`",
		},
		{
			search: `-"dog" -png -jumps`,
			desc:   "search for filepaths that don't have exactly `dog`, `png` and `jumps`",
		},
		{
			search: `dog "/cat" -mkv`,
			desc:   "search for filepaths that have the same prefixes as `dog`, have exactly `/cat` and don't have exactly `mkv`",
		},
		{
			search: `animals -"cat & dog"`,
			desc:   "search for filepaths that have the same prefixes as `animals` and don't have exactly `cat & dog`",
		},
	}

	rclone := &rcloneStub{
		GetAllFilesFn: func(ctx context.Context) ([]rclone.DirEntry, error) { return entries, nil },
	}
	s, err := NewService(rclone, root)
	r.NoError(err)
	err = s.Start()
	r.NoError(err)
	defer func() {
		err = s.Shutdown(t.Context())
		r.NoError(err)
	}()

	buf := bytes.NewBuffer(nil)

	fmt.Fprint(buf, "**Files:**\n\n")
	for _, f := range entries {
		fmt.Fprintf(buf, "- `%s`\n", f.URL)
	}

	fmt.Fprint(buf, "\n**Search Requests:**\n\n")
	for _, tt := range tests {
		hits, _, err := s.Search(t.Context(), tt.search, 10)
		r.NoError(err)

		fmt.Fprintf(buf, "- `%s` - %s. Results:\n", tt.search, tt.desc)
		for _, h := range hits {
			fmt.Fprintf(buf, "  - `%s`\n", h.Path)
		}
	}

	want, err := os.ReadFile("./testdata/docs.golden.md")
	r.NoError(err)
	r.Equal(string(want), buf.String())
}
