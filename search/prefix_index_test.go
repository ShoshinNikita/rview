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
		5: "gaming/",
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

		hits := index.Search(`games`, 5)
		r.Equal(
			[]Hit{
				{Path: "games/hi-fi rush/1.jpg", Score: 3},
				{Path: "games/hi-fi rush/2.jpg", Score: 3},
				{Path: "games/starfield/", Score: 3},
				{Path: "gaming/", Score: 1},
			},
			hits,
		)
	})

	t.Run("limit", func(t *testing.T) {
		r := require.New(t)

		hits := index.Search(`games`, 2)
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
		hits := index.Search(`games ru`, 5)
		r.Equal(
			[]Hit{
				{Path: "games/hi-fi rush/1.jpg", Score: 3},
				{Path: "games/hi-fi rush/2.jpg", Score: 3},
				{Path: "games/starfield/", Score: 3},
				{Path: "gaming/", Score: 1},
			},
			hits,
		)

		hits = index.Search(`games rush`, 5)
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

		hits := index.Search(`"games/hifi RUSH"`, 5)
		r.Empty(hits)

		hits = index.Search(`"games/hi-fi RUSH"`, 5)
		r.Equal(
			[]Hit{
				{Path: "games/hi-fi rush/1.jpg", Score: math.Inf(1)},
				{Path: "games/hi-fi rush/2.jpg", Score: math.Inf(1)},
			},
			hits,
		)

		hits = index.Search(`"games"`, 5)
		r.Equal(
			[]Hit{
				{Path: "games/hi-fi rush/1.jpg", Score: math.Inf(1)},
				{Path: "games/hi-fi rush/2.jpg", Score: math.Inf(1)},
				{Path: "games/starfield/", Score: math.Inf(1)},
			},
			hits,
		)

		hits = index.Search(`"games" "jpg"`, 5)
		r.Equal(
			[]Hit{
				{Path: "games/hi-fi rush/1.jpg", Score: math.Inf(1)},
				{Path: "games/hi-fi rush/2.jpg", Score: math.Inf(1)},
			},
			hits,
		)

		hits = index.Search(`"games" "jpg" "1"`, 5)
		r.Equal(
			[]Hit{
				{Path: "games/hi-fi rush/1.jpg", Score: math.Inf(1)},
			},
			hits,
		)

		hits = index.Search(`"games" "jpg" "1" "2"`, 5)
		r.Empty(hits)
	})

	t.Run("exclude", func(t *testing.T) {
		r := require.New(t)

		hits := index.Search(`games -"hi-fi"`, 5)
		r.Equal(
			[]Hit{
				{Path: "games/starfield/", Score: 3},
				{Path: "gaming/", Score: 1},
			},
			hits,
		)

		hits = index.Search(`games -"hi-fi" -"gaming"`, 5)
		r.Equal(
			[]Hit{
				{Path: "games/starfield/", Score: 3},
			},
			hits,
		)

		hits = index.Search(`"games" -"starfield"`, 5)
		r.Equal(
			[]Hit{
				{Path: "games/hi-fi rush/1.jpg", Score: math.Inf(1)},
				{Path: "games/hi-fi rush/2.jpg", Score: math.Inf(1)},
			},
			hits,
		)

		hits = index.Search(`-"games" -"gaming" -"лето"`, 5)
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
		hits := index.Search("a beautiful", 10)
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
		hits := index.Search("schuchternes", 10)
		r.Equal(
			[]Hit{{Path: "schüchternes Lächeln", Score: 5}},
			hits,
		)
		hits = index.Search("schüchternes", 10)
		r.Equal(
			[]Hit{{Path: "schüchternes Lächeln", Score: 5}},
			hits,
		)

		// But exact search succeeds only for input with accented characters.
		hits = index.Search(`"schuchternes"`, 10)
		r.Empty(hits)
		hits = index.Search(`"schüchternes"`, 10)
		r.NotEmpty(hits)

		// Other cases.
		hits = index.Search("hello", 10)
		r.Equal(
			[]Hit{
				{Path: "hello world", Score: 3},
				{Path: "ĥ̷̩e̴͕̯̺͛l̸̨̹͍̈́̍͛ḷ̵̬̗̓ô̴̝̯̈́", Score: 3},
			},
			hits,
		)
		hits = index.Search("ĥ̷̩e̴͕̯̺͛l̸̨̹͍̈́̍͛", 10)
		r.Equal(
			[]Hit{
				{Path: "hello world", Score: 1},
				{Path: "ĥ̷̩e̴͕̯̺͛l̸̨̹͍̈́̍͛ḷ̵̬̗̓ô̴̝̯̈́", Score: 1},
			},
			hits,
		)
		hits = index.Search("белыи", 10)
		r.Equal(
			[]Hit{{Path: "белый", Score: 3}},
			hits,
		)
		hits = index.Search("бёлый", 10)
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
		}
		index := newPrefixIndex(texts, 3, 7)

		hits := index.Search("anim", 10, searchOptions.CompactHits)
		r.Equal(
			[]Hit{
				{Path: "/animals/", Score: 2},
				{Path: "/anime/", Score: 2},
			},
			hits,
		)

		hits = index.Search("anim dogs", 10, searchOptions.CompactHits)
		r.Equal(
			[]Hit{
				{Path: "/animals/dogs/", Score: 4},
			},
			hits,
		)

		hits = index.Search("anim cats", 10, searchOptions.CompactHits)
		r.Equal(
			[]Hit{
				{Path: "/animals/cats/", Score: 4},
				{Path: "/animals/dogs/2025/catch/", Score: 3},
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
				searchForWords: `nothi--n"g to -exclude`,
			},
		},
		{
			search: `hello dear -"word-abc" -""`,
			want: searchRequest{
				toExclude:      []string{"word-abc"},
				searchForWords: `hello dear  -""`,
			},
		},
		{
			search: `-"test!&some--/characters" -"animal park"`,
			want: searchRequest{
				toExclude:      []string{"test!&some--/characters", "animal park"},
				searchForWords: "",
			},
		},
		{
			search: `"exact" "match"`,
			want: searchRequest{
				exactMatches:   []string{"exact", "match"},
				searchForWords: "",
			},
		},
		{
			search: `-"first" test -"second" abc "third" qwerty`,
			want: searchRequest{
				toExclude:      []string{"first", "second"},
				exactMatches:   []string{"third"},
				searchForWords: "test  abc  qwerty",
			},
		},
		{
			search: `-"inside-"""`,
			want: searchRequest{
				toExclude:      []string{"inside-"},
				searchForWords: `""`,
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
