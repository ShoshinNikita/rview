package index

import (
	"encoding/json"
	"io/fs"
	"math"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ShoshinNikita/rview/pkg/require"
	"github.com/ShoshinNikita/rview/rclone"
)

func TestPrefixIndex(t *testing.T) {
	r := require.New(t)

	entries := [...]*dirEntry{
		0: {Path: "/hello !&a! world.go"},
		1: {Path: "/games/starfield/", IsDir: true},
		2: {Path: "/games/hi-fi rush/1.jpg"},
		3: {Path: "/games/hi-fi rush/2.jpg"},
		4: {Path: "/изображения/лето 2022/", IsDir: true},
		5: {Path: "/gaming/", IsDir: true},
	}
	index := newPrefixIndex(slices.Values(entries[:]), 3, 7)
	r.Equal(
		map[string][]uint32{
			"hel":   {0},
			"hell":  {0},
			"hello": {0},
			//
			"wor":   {0},
			"worl":  {0},
			"world": {0},
			//
			"gam":   {1, 2, 3, 5},
			"game":  {1, 2, 3},
			"games": {1, 2, 3},
			//
			"sta":     {1},
			"star":    {1},
			"starf":   {1},
			"starfi":  {1},
			"starfie": {1},
			//
			"hif":  {2, 3},
			"hifi": {2, 3},
			//
			"rus":  {2, 3},
			"rush": {2, 3},
			//
			"jpg": {2, 3},
			//
			"изо":     {4},
			"изоб":    {4},
			"изобр":   {4},
			"изобра":  {4},
			"изображ": {4},
			//
			"лет":  {4},
			"лето": {4},
			//
			"202":  {4},
			"2022": {4},
			//
			"gami":   {5},
			"gamin":  {5},
			"gaming": {5},
		},
		index.Prefixes,
	)

	// Test Marshal/Unmarshal first to check that it doesn't affect search.
	t.Run("unmarshal", func(t *testing.T) {
		r := require.New(t)

		rawIndex, err := json.Marshal(index)
		r.NoError(err)

		index = nil

		err = json.Unmarshal(rawIndex, &index)
		r.NoError(err)

		r.NotEqual(0, len(index.lowerCasedPaths))
	})

	t.Run("basic search", func(t *testing.T) {
		r := require.New(t)

		hits, _ := index.Search(`games`, 5)
		r.Equal(
			[]Hit{
				{Path: "/games/starfield/", Score: 3, IsDir: true},
				{Path: "/games/hi-fi rush/1.jpg", Score: 3},
				{Path: "/games/hi-fi rush/2.jpg", Score: 3},
				{Path: "/gaming/", Score: 1, IsDir: true},
			},
			hits,
		)
	})

	t.Run("limit", func(t *testing.T) {
		r := require.New(t)

		hits, total := index.Search(`games`, 2)
		r.Equal(4, total)
		r.Equal(
			[]Hit{
				{Path: "/games/starfield/", Score: 3, IsDir: true},
				{Path: "/games/hi-fi rush/1.jpg", Score: 3},
			},
			hits,
		)
	})

	t.Run("multiple words", func(t *testing.T) {
		r := require.New(t)

		// Short words must be ignored
		hits, _ := index.Search(`games ru`, 5)
		r.Equal(
			[]Hit{
				{Path: "/games/starfield/", Score: 3, IsDir: true},
				{Path: "/games/hi-fi rush/1.jpg", Score: 3},
				{Path: "/games/hi-fi rush/2.jpg", Score: 3},
				{Path: "/gaming/", Score: 1, IsDir: true},
			},
			hits,
		)

		hits, _ = index.Search(`games rush`, 5)
		r.Equal(
			[]Hit{
				{Path: "/games/hi-fi rush/1.jpg", Score: 5},
				{Path: "/games/hi-fi rush/2.jpg", Score: 5},
			},
			hits,
		)
	})

	t.Run("exact match", func(t *testing.T) {
		r := require.New(t)

		hits, _ := index.Search(`"games/hifi RUSH"`, 5)
		r.Len(hits, 0)

		hits, _ = index.Search(`"games/hi-fi RUSH"`, 5)
		r.Equal(
			[]Hit{
				{Path: "/games/hi-fi rush/1.jpg", Score: float32(math.Inf(1))},
				{Path: "/games/hi-fi rush/2.jpg", Score: float32(math.Inf(1))},
			},
			hits,
		)

		hits, _ = index.Search(`"games"`, 5)
		r.Equal(
			[]Hit{
				{Path: "/games/starfield/", Score: float32(math.Inf(1)), IsDir: true},
				{Path: "/games/hi-fi rush/1.jpg", Score: float32(math.Inf(1))},
				{Path: "/games/hi-fi rush/2.jpg", Score: float32(math.Inf(1))},
			},
			hits,
		)

		hits, _ = index.Search(`"games" "jpg"`, 5)
		r.Equal(
			[]Hit{
				{Path: "/games/hi-fi rush/1.jpg", Score: float32(math.Inf(1))},
				{Path: "/games/hi-fi rush/2.jpg", Score: float32(math.Inf(1))},
			},
			hits,
		)

		hits, _ = index.Search(`"games" "jpg" "1"`, 5)
		r.Equal(
			[]Hit{
				{Path: "/games/hi-fi rush/1.jpg", Score: float32(math.Inf(1))},
			},
			hits,
		)

		hits, _ = index.Search(`"games" "jpg" "1" "2"`, 5)
		r.Len(hits, 0)
	})

	t.Run("exclude", func(t *testing.T) {
		r := require.New(t)

		hits, _ := index.Search(`games -"hi-fi"`, 5)
		r.Equal(
			[]Hit{
				{Path: "/games/starfield/", Score: 3, IsDir: true},
				{Path: "/gaming/", Score: 1, IsDir: true},
			},
			hits,
		)

		hits, _ = index.Search(`games -"hi-fi" -"gaming"`, 5)
		r.Equal(
			[]Hit{
				{Path: "/games/starfield/", Score: 3, IsDir: true},
			},
			hits,
		)

		hits, _ = index.Search(`"games" -"starfield"`, 5)
		r.Equal(
			[]Hit{
				{Path: "/games/hi-fi rush/1.jpg", Score: float32(math.Inf(1))},
				{Path: "/games/hi-fi rush/2.jpg", Score: float32(math.Inf(1))},
			},
			hits,
		)

		hits, _ = index.Search(`-"games" -"gaming" -"лето"`, 5)
		r.Equal(
			[]Hit{
				{Path: "/hello !&a! world.go", Score: float32(math.Inf(1))},
			},
			hits,
		)
	})

	t.Run("search with a one-letter word", func(t *testing.T) {
		r := require.New(t)

		entries := []*dirEntry{
			{Path: "a beautiful picture"},
		}
		index := newPrefixIndex(slices.Values(entries), 3, 7)
		hits, _ := index.Search("a beautiful", 10)
		r.Equal(
			[]Hit{
				{Path: "a beautiful picture", Score: 5},
			},
			hits,
		)
		hits, _ = index.Search("a b beautiful", 10)
		r.Equal(
			[]Hit{
				{Path: "a beautiful picture", Score: 5},
			},
			hits,
		)
		hits, _ = index.Search(`a "beautiful"`, 10)
		r.Equal(
			[]Hit{
				{Path: "a beautiful picture", Score: float32(math.Inf(1))},
			},
			hits,
		)
	})

	t.Run("unicode", func(t *testing.T) {
		r := require.New(t)

		entries := []*dirEntry{
			{Path: "schüchternes Lächeln"},
			{Path: "hello world"},
			{Path: "ĥ̷̩e̴͕̯̺͛l̸̨̹͍̈́̍͛ḷ̵̬̗̓ô̴̝̯̈́"}, // hello
			{Path: "белый"},
		}
		index := newPrefixIndex(slices.Values(entries), 3, 7)

		// Both searches, with and without accented characters, succeed.
		hits, _ := index.Search("schuchternes", 10)
		r.Equal(
			[]Hit{{Path: "schüchternes Lächeln", Score: 5}},
			hits,
		)
		hits, _ = index.Search("schüchternes", 10)
		r.Equal(
			[]Hit{{Path: "schüchternes Lächeln", Score: 5}},
			hits,
		)

		// But exact search succeeds only for input with accented characters.
		hits, _ = index.Search(`"schuchternes"`, 10)
		r.Len(hits, 0)
		hits, _ = index.Search(`"schüchternes"`, 10)
		r.NotEqual(0, len(hits))

		// Other cases.
		hits, _ = index.Search("hello", 10)
		r.Equal(
			[]Hit{
				{Path: "hello world", Score: 3},
				{Path: "ĥ̷̩e̴͕̯̺͛l̸̨̹͍̈́̍͛ḷ̵̬̗̓ô̴̝̯̈́", Score: 3},
			},
			hits,
		)
		hits, _ = index.Search("ĥ̷̩e̴͕̯̺͛l̸̨̹͍̈́̍͛", 10)
		r.Equal(
			[]Hit{
				{Path: "hello world", Score: 1},
				{Path: "ĥ̷̩e̴͕̯̺͛l̸̨̹͍̈́̍͛ḷ̵̬̗̓ô̴̝̯̈́", Score: 1},
			},
			hits,
		)
		hits, _ = index.Search("белыи", 10)
		r.Equal(
			[]Hit{{Path: "белый", Score: 3}},
			hits,
		)
		hits, _ = index.Search("бёлый", 10)
		r.Equal(
			[]Hit{{Path: "белый", Score: 3}},
			hits,
		)
	})

	t.Run("compact hits", func(t *testing.T) {
		r := require.New(t)

		entries := []*dirEntry{
			{Path: "/animals/", IsDir: true},
			{Path: "/animals/cats/", IsDir: true},
			{Path: "/animals/dogs/", IsDir: true},
			{Path: "/animals/dogs/2025/catch/", IsDir: true},
			{Path: "/animals/dogs/2025/dog park/", IsDir: true},
			{Path: "/anime/", IsDir: true},
			{Path: "/anime/art.jpeg"},
		}
		index := newPrefixIndex(slices.Values(entries), 3, 7)

		hits, _ := index.Search("anim", 10)
		r.Equal(
			[]Hit{
				{Path: "/animals/", Score: 2, IsDir: true},
				{Path: "/anime/", Score: 2, IsDir: true},
			},
			hits,
		)

		hits, _ = index.Search("anim dogs", 10)
		r.Equal(
			[]Hit{
				{Path: "/animals/dogs/", Score: 4, IsDir: true},
			},
			hits,
		)

		hits, _ = index.Search("anim cats", 10)
		r.Equal(
			[]Hit{
				{Path: "/animals/cats/", Score: 4, IsDir: true},
				{Path: "/animals/dogs/2025/catch/", Score: 3, IsDir: true},
			},
			hits,
		)

		hits, _ = index.Search("anime jpeg", 10)
		r.Equal(
			[]Hit{
				{Path: "/anime/art.jpeg", Score: 5},
			},
			hits,
		)

		// 2 hits because '/game/' and '/game/gamesaves/' have different scores.
		entries = []*dirEntry{
			{Path: "/game/", IsDir: true},
			{Path: "/game/inputs.txt"},
			{Path: "/game/config.txt"},
			{Path: "/game/gamesaves/", IsDir: true},
			{Path: "/game/gamesaves/1.txt"},
			{Path: "/game/gamesaves/2.txt"},
			{Path: "/game/gamesaves/3.txt"},
		}
		index = newPrefixIndex(slices.Values(entries), 3, 7)
		hits, _ = index.Search("games", 10)
		r.Equal(
			[]Hit{
				{Path: "/game/gamesaves/", Score: 3, IsDir: true},
				{Path: "/game/", Score: 2, IsDir: true},
			},
			hits,
		)

		// Don't merge 'test.Dockerfile' and 'test.Dockerfile.dockerignore'
		entries = []*dirEntry{
			{Path: "/Dockerfile"},
			{Path: "/test.Dockerfile"},
			{Path: "/test.Dockerfile.dockerignore"},
		}
		index = newPrefixIndex(slices.Values(entries), 3, 7)
		hits, _ = index.Search(`"dockerfile"`, 10)
		r.Equal(
			[]Hit{
				{Path: "/Dockerfile", Score: float32(math.Inf(1))},
				{Path: "/test.Dockerfile", Score: float32(math.Inf(1))},
				{Path: "/test.Dockerfile.dockerignore", Score: float32(math.Inf(1))},
			},
			hits,
		)
	})

	t.Run("metadata", func(t *testing.T) {
		entries := []*dirEntry{
			{Path: "/cats/", ModTime: 123, IsDir: true},
			{Path: "/animals/cat.jpeg", Size: 1 << 20, ModTime: 124},
			{Path: "/animals/cute/cats.png", Size: 1 << 13, ModTime: 130},
		}
		index := newPrefixIndex(slices.Values(entries), 3, 7)

		hits, _ := index.Search("cat", 10)
		r.Equal(
			[]Hit{
				{Path: "/cats/", IsDir: true, Size: 0, ModTime: 123, Score: 1},
				{Path: "/animals/cat.jpeg", Size: 1 << 20, ModTime: 124, Score: 1},
				{Path: "/animals/cute/cats.png", Size: 1 << 13, ModTime: 130, Score: 1},
			},
			hits,
		)
	})
}

func TestNewSearchRequest(t *testing.T) {
	for _, tt := range []struct {
		search     string
		want       searchRequest
		checkWords bool
	}{
		{
			search: `nothi--n"g to -exclude`,
			want: searchRequest{
				toExclude:      []string{"exclude"},
				extractedWords: []string{`nothi--n"g`, "to"},
			},
		},
		{
			search: `hello dear -"word-abc" -""`,
			want: searchRequest{
				toExclude:      []string{"word-abc"},
				extractedWords: []string{"hello", "dear"},
			},
		},
		{
			search: `-"test!&some--/characters" -"animal park"`,
			want: searchRequest{
				toExclude: []string{"test!&some--/characters", "animal park"},
			},
		},
		{
			search: `"exact" "match"`,
			want: searchRequest{
				exactMatches: []string{"exact", "match"},
			},
		},
		{
			search: `-"first" test -"second" abc "third" qwerty`,
			want: searchRequest{
				toExclude:      []string{"first", "second"},
				exactMatches:   []string{"third"},
				extractedWords: []string{"test", "abc", "qwerty"},
			},
		},
		{
			search: `-"inside-"""`,
			want: searchRequest{
				toExclude: []string{"inside-"},
			},
		},
		{
			search: `hello -"  beautiful world"  ""   abc "def"hh  -xxx --qwerty test"xy"z aa"a f   `,
			want: searchRequest{
				toExclude:      []string{"beautiful world", "xxx", "-qwerty"},
				exactMatches:   []string{"def"},
				extractedWords: []string{"hello", "abc", "hh", `test"xy"z`, `aa"a`, "f"},
			},
		},
		{
			search: `-"hello world     `,
			want: searchRequest{
				toExclude: []string{"hello world"},
			},
		},
		{
			search: `a/aa/aaa/aaaa`,
			want: searchRequest{
				words: [][]rune{
					[]rune("aaa"),
					[]rune("aaaa"),
				},
				extractedWords: []string{"a/aa/aaa/aaaa"},
			},
			checkWords: true,
		},
		{
			search: `a b c dd`,
			want: searchRequest{
				extractedWords: []string{"a", "b", "c", "dd"},
			},
			checkWords: true,
		},
	} {
		t.Run("", func(t *testing.T) {
			got := newSearchRequest(tt.search, 3)
			if !tt.checkWords {
				got.words = nil // too tiresome to test
			}

			require.New(t).Equal(tt.want, got)
		})
	}
}

func TestFloat32Inf(t *testing.T) {
	r := require.New(t)

	f := float32(math.Inf(1))
	r.True(f > math.MaxFloat32)
	r.True(math.IsInf(float64(f), 0))
}

func newDirEntry(p string) rclone.DirEntry {
	return newDirEntryWithMetadata(p, 0, 0)
}

func newDirEntryWithMetadata(p string, size, modTime int64) rclone.DirEntry {
	return rclone.DirEntry{
		URL:     p,
		Leaf:    path.Base(p),
		IsDir:   strings.HasSuffix(p, "/"),
		Size:    size,
		ModTime: modTime,
	}
}

func BenchmarkPrefixIndex_Search(b *testing.B) {
	var entries []*dirEntry
	err := filepath.WalkDir("..", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		entries = append(entries, &dirEntry{
			Path:    p,
			IsDir:   d.IsDir(),
			Size:    0,
			ModTime: 0,
		})
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}

	b.Logf("%d entries have been loaded", len(entries))

	index := newPrefixIndex(slices.Values(entries), 3, 10)

	run := func(s string) {
		b.Run(s, func(b *testing.B) {
			for b.Loop() {
				index.Search(s, 10)
			}
		})
	}
	run(`vendor`)
	run(`vendor -github`)
	run(`"vendor" -github`)
	run(`"vendor" github google`)
	run(`rview`)
	run(`Dockerfile`)
	run(`"Dockerfile"`)
}

func (r searchRequest) Equal(r2 searchRequest) bool {
	return reflect.DeepEqual(r, r2)
}
