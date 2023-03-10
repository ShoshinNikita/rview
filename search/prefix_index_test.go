package search

import (
	"math"
	"testing"

	"github.com/ShoshinNikita/rview/rview"
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

	t.Run("basic search", func(t *testing.T) {
		r := require.New(t)

		hits := index.Search(`games`, 5)
		r.Equal(
			[]rview.SearchHit{
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
			[]rview.SearchHit{
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
			[]rview.SearchHit{
				{Path: "games/hi-fi rush/1.jpg", Score: 3},
				{Path: "games/hi-fi rush/2.jpg", Score: 3},
				{Path: "games/starfield/", Score: 3},
				{Path: "gaming/", Score: 1},
			},
			hits,
		)

		hits = index.Search(`games rush`, 5)
		r.Equal(
			[]rview.SearchHit{
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
			[]rview.SearchHit{
				{Path: "games/hi-fi rush/1.jpg", Score: math.Inf(1)},
				{Path: "games/hi-fi rush/2.jpg", Score: math.Inf(1)},
			},
			hits,
		)
	})
}
