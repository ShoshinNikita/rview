package search

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrefixIndex(t *testing.T) {
	r := require.New(t)

	texts := [...]string{
		0: "hello !&a! world.go",
		1: "games/starfield/",
		2: "games/hi-fi rush/1.jpg",
		3: "games/hi-fi rush/2.jpg",
		4: "изображения/лето 2022/",
		5: "/gaming/", // only trailing '/'s matter
	}
	index := newPrefixIndex(texts[:], 3, 7)
	r.Equal(
		map[string][]uint64{
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

		r.NotEmpty(index.lowerCasedPaths)
	})

	t.Run("basic search", func(t *testing.T) {
		r := require.New(t)

		hits, _ := index.Search(`games`, 5)
		r.Equal(
			[]Hit{
				{Path: "games/hi-fi rush/1.jpg", Score: 3},
				{Path: "games/hi-fi rush/2.jpg", Score: 3},
				{Path: "games/starfield/", Score: 3, IsDir: true},
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
				{Path: "games/hi-fi rush/1.jpg", Score: 3},
				{Path: "games/hi-fi rush/2.jpg", Score: 3},
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
				{Path: "games/hi-fi rush/1.jpg", Score: 3},
				{Path: "games/hi-fi rush/2.jpg", Score: 3},
				{Path: "games/starfield/", Score: 3, IsDir: true},
				{Path: "/gaming/", Score: 1, IsDir: true},
			},
			hits,
		)

		hits, _ = index.Search(`games rush`, 5)
		r.Equal(
			[]Hit{
				{Path: "games/hi-fi rush/1.jpg", Score: 5},
				{Path: "games/hi-fi rush/2.jpg", Score: 5},
			},
			hits,
		)
	})

	t.Run("exact match", func(t *testing.T) {
		r := require.New(t)

		hits, _ := index.Search(`"games/hifi RUSH"`, 5)
		r.Empty(hits)

		hits, _ = index.Search(`"games/hi-fi RUSH"`, 5)
		r.Equal(
			[]Hit{
				{Path: "games/hi-fi rush/1.jpg", Score: math.Inf(1)},
				{Path: "games/hi-fi rush/2.jpg", Score: math.Inf(1)},
			},
			hits,
		)

		hits, _ = index.Search(`"games"`, 5)
		r.Equal(
			[]Hit{
				{Path: "games/hi-fi rush/1.jpg", Score: math.Inf(1)},
				{Path: "games/hi-fi rush/2.jpg", Score: math.Inf(1)},
				{Path: "games/starfield/", Score: math.Inf(1), IsDir: true},
			},
			hits,
		)

		hits, _ = index.Search(`"games" "jpg"`, 5)
		r.Equal(
			[]Hit{
				{Path: "games/hi-fi rush/1.jpg", Score: math.Inf(1)},
				{Path: "games/hi-fi rush/2.jpg", Score: math.Inf(1)},
			},
			hits,
		)

		hits, _ = index.Search(`"games" "jpg" "1"`, 5)
		r.Equal(
			[]Hit{
				{Path: "games/hi-fi rush/1.jpg", Score: math.Inf(1)},
			},
			hits,
		)

		hits, _ = index.Search(`"games" "jpg" "1" "2"`, 5)
		r.Empty(hits)
	})

	t.Run("exclude", func(t *testing.T) {
		r := require.New(t)

		hits, _ := index.Search(`games -"hi-fi"`, 5)
		r.Equal(
			[]Hit{
				{Path: "games/starfield/", Score: 3, IsDir: true},
				{Path: "/gaming/", Score: 1, IsDir: true},
			},
			hits,
		)

		hits, _ = index.Search(`games -"hi-fi" -"gaming"`, 5)
		r.Equal(
			[]Hit{
				{Path: "games/starfield/", Score: 3, IsDir: true},
			},
			hits,
		)

		hits, _ = index.Search(`"games" -"starfield"`, 5)
		r.Equal(
			[]Hit{
				{Path: "games/hi-fi rush/1.jpg", Score: math.Inf(1)},
				{Path: "games/hi-fi rush/2.jpg", Score: math.Inf(1)},
			},
			hits,
		)

		hits, _ = index.Search(`-"games" -"gaming" -"лето"`, 5)
		r.Equal(
			[]Hit{
				{Path: "hello !&a! world.go", Score: math.Inf(1)},
			},
			hits,
		)
	})

	t.Run("search with a one-letter word", func(t *testing.T) {
		r := require.New(t)

		index := newPrefixIndex([]string{"a beautiful picture"}, 3, 7)
		hits, _ := index.Search("a beautiful", 10)
		r.Equal(
			[]Hit{
				{Path: "a beautiful picture", Score: 5},
			},
			hits,
		)
	})

	t.Run("unicode", func(t *testing.T) {
		r := require.New(t)

		texts := []string{
			"schüchternes Lächeln",
			"hello world",
			"ĥ̷̩e̴͕̯̺͛l̸̨̹͍̈́̍͛ḷ̵̬̗̓ô̴̝̯̈́", // hello
			"белый",
		}
		index := newPrefixIndex(texts, 3, 7)

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
		r.Empty(hits)
		hits, _ = index.Search(`"schüchternes"`, 10)
		r.NotEmpty(hits)

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

		texts := []string{
			"/animals/",
			"/animals/cats/",
			"/animals/dogs/",
			"/animals/dogs/2025/catch/",
			"/animals/dogs/2025/dog park/",
			"/anime/",
			"/anime/art.jpeg",
		}
		index := newPrefixIndex(texts, 3, 7)

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
	})
}

func TestNewSearchRequest(t *testing.T) {
	for _, tt := range []struct {
		search string
		want   searchRequest
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
	} {
		t.Run("", func(t *testing.T) {
			got := newSearchRequest(tt.search)
			got.words = nil // too tiresome to test

			assert.Equal(t, tt.want, got)
		})
	}
}
