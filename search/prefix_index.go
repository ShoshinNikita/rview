package search

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/ShoshinNikita/rview/rview"
)

type prefixIndex struct {
	MinPrefixLen int                 `json:"min_prefix_len"`
	MaxPrefixLen int                 `json:"max_prefix_len"`
	Paths        map[uint64]string   `json:"paths"`
	Prefixes     map[string][]uint64 `json:"prefixes"`
}

func newPrefixIndex(rawPaths []string, minPrefixLen, maxPrefixLen int) *prefixIndex {
	paths := make(map[uint64]string, len(rawPaths))
	for i, path := range rawPaths {
		paths[uint64(i)] = path
	}

	prefixes := make(map[string][]uint64)
	for id, path := range paths {
		for _, prefix := range generatePrefixes(path, minPrefixLen, maxPrefixLen) {
			prefixes[prefix] = append(prefixes[prefix], id)
		}
	}
	for prefix, ids := range prefixes {
		ids = unique(ids)
		sort.Slice(ids, func(i, j int) bool {
			return ids[i] < ids[j]
		})
		prefixes[prefix] = ids
	}

	return &prefixIndex{
		MinPrefixLen: minPrefixLen,
		MaxPrefixLen: maxPrefixLen,
		Paths:        paths,
		Prefixes:     prefixes,
	}
}

func (index *prefixIndex) Check(wantMin, wantMax int) error {
	if index.MinPrefixLen != wantMin || index.MaxPrefixLen != wantMax {
		return fmt.Errorf(
			"prefix sizes are different: [%d; %d] (index) != [%d; %d] (expected)",
			index.MinPrefixLen, index.MaxPrefixLen,
			wantMin, wantMax,
		)
	}
	return nil
}

func (index *prefixIndex) Search(search string, limit int) (hits []rview.SearchHit) {
	// Check whether we should search for the exact match.
	if strings.Count(search, `"`) == 2 && strings.HasPrefix(search, `"`) && strings.HasSuffix(search, `"`) {
		// Trim leading and trailing '"'.
		search := search[1 : len(search)-1]
		hits = index.searchExact(search)

	} else {
		hits = index.search(search)
	}

	sort.Slice(hits, func(i, j int) bool {
		a := hits[i]
		b := hits[j]
		if a.Score == b.Score {
			return a.Path < b.Path
		}
		return a.Score > b.Score
	})

	if len(hits) > limit {
		hits = hits[:limit]
	}

	return hits
}

// searchExact searches for exact matches.
func (index *prefixIndex) searchExact(search string) []rview.SearchHit {
	search = strings.ToLower(search)

	var res []rview.SearchHit
	for _, path := range index.Paths {
		if strings.Contains(strings.ToLower(path), search) {
			res = append(res, rview.SearchHit{
				Path:  path,
				Score: math.Inf(1),
			})
		}
	}
	return res
}

// search searches for prefix matches. If passed "search" contains multiple words,
// only results that matched all these words will be returned.
func (index *prefixIndex) search(search string) []rview.SearchHit {
	words := splitToNormalizedWords(search)

	var (
		matchCounts        = make(map[uint64]int)
		matchesForAllWords = make(map[uint64]bool)
	)
	for i, word := range words {
		// If a word length is less than MinPrefixLen, no prefixes will be generated, and
		// no hits will be returned. So, ignore such words.
		if len(word) < index.MinPrefixLen {
			continue
		}

		matches := make(map[uint64]bool)
		for _, prefix := range generatePrefixes(string(word), index.MinPrefixLen, index.MaxPrefixLen) {
			for _, id := range index.Prefixes[prefix] {
				matchCounts[id]++

				matches[id] = true
			}
		}

		// No reason to continue search.
		if len(matches) == 0 {
			return nil
		}

		if i == 0 {
			// Fill matchesForAllWords on first word.
			for k, v := range matches {
				matchesForAllWords[k] = v
			}
		} else {
			// All words have to match.
			for k := range matchesForAllWords {
				if !matches[k] {
					delete(matchesForAllWords, k)
				}
			}
		}
	}

	hits := make([]rview.SearchHit, 0, len(matchesForAllWords))
	for id := range matchesForAllWords {
		path := index.Paths[id]
		count := matchCounts[id]

		hits = append(hits, rview.SearchHit{
			Path:  path,
			Score: float64(count),
		})
	}
	return hits
}

func generatePrefixes(path string, minLen, maxLen int) (prefixes []string) {
	words := splitToNormalizedWords(path)
	for _, word := range words {
		for i := minLen; i <= maxLen; i++ {
			if i > len(word) {
				break
			}
			prefixes = append(prefixes, string(word[:i]))
		}
	}
	return prefixes
}

func splitToNormalizedWords(v string) (res [][]rune) {
	var (
		word       []rune
		appendWord = func() {
			if len(word) > 0 {
				res = append(res, word)
				word = []rune{}
			}
		}
	)
	for _, r := range v {
		switch {
		case unicode.IsDigit(r), unicode.IsLetter(r):
			word = append(word, unicode.ToLower(r))
		case unicode.IsSpace(r) || r == '/' || r == '.':
			appendWord()
		}
	}
	appendWord()

	return res
}

func unique(slice []uint64) (res []uint64) {
	seen := make(map[uint64]bool)
	for _, v := range slice {
		if !seen[v] {
			seen[v] = true
			res = append(res, v)
		}
	}
	return res
}
