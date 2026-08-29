package index

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"iter"
	"os"
	"slices"
	"testing"

	"github.com/ShoshinNikita/rview/pkg/require"
	"github.com/ShoshinNikita/rview/rclone"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/goccy/go-yaml"
)

func TestService_RefreshIndex(t *testing.T) {
	r := require.New(t)
	ctx := t.Context()

	root, err := os.OpenRoot(t.TempDir())
	r.NoError(err)

	rcloneStub := &rcloneStub{
		GetAllFilesFn: func(context.Context) (iter.Seq[rclone.DirEntry], error) {
			return slices.Values([]rclone.DirEntry{
				newDirEntry("/"),
				newDirEntry("/hello world.go"),
				newDirEntry("/gaming.txt"),
				newDirEntry("/.rview.yml"),
				newDirEntry("/arts/"),
				newDirEntry("/arts/games/"),
				newDirEntry("/arts/games/1.jpeg"),
			}), nil
		},
		OpenFileFn: func(ctx context.Context, id rview.FileID) (io.ReadCloser, error) {
			switch id.GetPath() {
			case "/.rview.yml":
				data, err := yaml.Marshal(dirMetaInfo{
					DefaultSort: "time_desc",
					Annotations: map[string]string{
						".":     "root",
						"*.txt": "text",
						"arts":  "pictures",
						"arts/": "paintings",
					},
				})
				return io.NopCloser(bytes.NewReader(data)), err
			default:
				return nil, fmt.Errorf("unexpected path: %q", id.GetPath())
			}
		},
	}
	s, err := NewService(rcloneStub, root)
	r.NoError(err)

	// File doesn't exist.
	_, err = s.loadIndexFromFile()
	r.Error(err)

	err = s.Start()
	r.NoError(err)
	defer func() {
		err := s.Shutdown(t.Context())
		r.NoError(err)
	}()

	// Basic search.
	hits, _, err := s.Search(ctx, "games", 5)
	r.NoError(err)
	r.Equal(
		[]Hit{
			{Path: "/arts/games/", IsDir: true, Score: 3},
			{Path: "/gaming.txt", IsDir: false, Score: 1},
		},
		hits,
	)

	// Search by extra search terms.
	hits, _, err = s.Search(ctx, "pictures", 5)
	r.NoError(err)
	r.Equal([]Hit{{Path: "/arts/", IsDir: true, Score: 6}}, hits)
	//
	hits, _, err = s.Search(ctx, "paintings", 5)
	r.NoError(err)
	r.Equal([]Hit{{Path: "/arts/", IsDir: true, Score: 7}}, hits)
	//
	hits, _, err = s.Search(ctx, "root", 5)
	r.NoError(err)
	r.Equal([]Hit{{Path: "/", IsDir: true, Score: 2}}, hits)
	//
	hits, _, err = s.Search(ctx, "text", 5)
	r.NoError(err)
	r.Equal([]Hit{{Path: "/gaming.txt", Score: 2}}, hits)

	// File should be created.
	searchIndex, err := s.loadIndexFromFile()
	r.NoError(err)
	hits, _ = searchIndex.Index.Search("games", 5)
	r.Len(hits, 2)

	// Refresh index.
	rcloneStub.GetAllFilesFn = func(context.Context) (iter.Seq[rclone.DirEntry], error) {
		return slices.Values([]rclone.DirEntry{
			newDirEntry("/hello world.go"),
			newDirEntry("/qwerty.txt"),
		}), nil
	}
	err = s.RefreshIndex(ctx)
	r.NoError(err)

	hits, _, err = s.Search(ctx, "games", 5)
	r.NoError(err)
	r.Len(hits, 0)

	searchIndex, err = s.loadIndexFromFile()
	r.NoError(err)
	hits, _ = searchIndex.Index.Search("games", 5)
	r.Len(hits, 0)
}

type rcloneStub struct {
	GetAllFilesFn func(context.Context) (iter.Seq[rclone.DirEntry], error)
	OpenFileFn    func(ctx context.Context, id rview.FileID) (io.ReadCloser, error)
}

func (s rcloneStub) GetAllFiles(ctx context.Context) (iter.Seq[rclone.DirEntry], error) {
	return s.GetAllFilesFn(ctx)
}

func (s rcloneStub) OpenFile(ctx context.Context, id rview.FileID) (io.ReadCloser, error) {
	return s.OpenFileFn(ctx, id)
}

// TestService_GenerateDocs generates an output in Markdown format that is used in documentation for search.
func TestGenerateDocs(t *testing.T) {
	r := require.New(t)

	root, err := os.OpenRoot(t.TempDir())
	r.NoError(err)

	entries := []rclone.DirEntry{
		newDirEntry("/animals/cute cat.jpeg"),
		newDirEntry("/animals/cat jumps.mp4"),
		newDirEntry("/animals/caterpillar.png"),
		newDirEntry("/animals/Cat & Dog play.mkv"),
		newDirEntry("/dogmas/catalog.zip"),
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
		GetAllFilesFn: func(ctx context.Context) (iter.Seq[rclone.DirEntry], error) { return slices.Values(entries), nil },
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
