package search

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

type Hit struct {
	Path  string
	IsDir bool
	Score float64
}

func (h Hit) compare(h1 Hit) int {
	if h.Score != h1.Score {
		return -1 * cmp.Compare(h.Score, h1.Score)
	}
	return cmp.Compare(h.Path, h1.Path)
}

type prefixIndex struct {
	MinPrefixLen int                 `json:"min_prefix_len"`
	MaxPrefixLen int                 `json:"max_prefix_len"`
	Paths        map[uint64]string   `json:"paths"`
	Prefixes     map[string][]uint64 `json:"prefixes"`

	lowerCasedPaths map[uint64]string
}

func newPrefixIndex(rawPaths []string, minPrefixLen, maxPrefixLen int) *prefixIndex {
	var (
		paths    = make(map[uint64]string, len(rawPaths))
		prefixes = make(map[string][]uint64)
	)
	for i, path := range rawPaths {
		id := uint64(i) //nolint:gosec

		paths[id] = path
		for _, prefix := range generatePrefixes(path, minPrefixLen, maxPrefixLen) {
			prefixes[prefix] = append(prefixes[prefix], id)
		}
	}
	for prefix, ids := range prefixes {
		slices.Sort(ids)
		prefixes[prefix] = slices.Clone(slices.Compact(ids)) // use slices.Clone because we don't need extra capacity
	}

	index := &prefixIndex{
		MinPrefixLen: minPrefixLen,
		MaxPrefixLen: maxPrefixLen,
		Paths:        paths,
		Prefixes:     prefixes,
	}
	index.prepare()

	return index
}

func (index *prefixIndex) UnmarshalJSON(data []byte) error {
	type tmpType prefixIndex

	var tmp tmpType
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}

	*index = prefixIndex(tmp)
	index.prepare()
	return nil
}

func (index *prefixIndex) prepare() {
	index.lowerCasedPaths = make(map[uint64]string)
	for id, path := range index.Paths {
		index.lowerCasedPaths[id] = strings.ToLower(path)
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

type searchHit struct {
	id             uint64
	lowerCasedPath string
	score          float64
}

func (index *prefixIndex) Search(search string, limit int) ([]Hit, int) {
	req := newSearchRequest(search)
	if len(req.words) == 0 && len(req.exactMatches) == 0 && len(req.toExclude) == 0 {
		// Just in case
		return nil, 0
	}

	var hits []searchHit
	if len(req.words) > 0 {
		hits = index.searchByPrefixes(req.words)

	} else {
		// Only exact matches, only excludes, or both - have to check all paths.
		hits = make([]searchHit, 0, len(index.Paths))
		for id := range index.Paths {
			hits = append(hits, index.newSearchHit(id, math.Inf(1)))
		}
	}

	// Filter by exact matches.
	if len(req.exactMatches) > 0 {
		hits = slices.DeleteFunc(hits, func(h searchHit) bool {
			for _, exact := range req.exactMatches {
				if !strings.Contains(h.lowerCasedPath, exact) {
					return true
				}
			}
			return false
		})
	}

	// Filter by excludes.
	if len(req.toExclude) > 0 {
		hits = slices.DeleteFunc(hits, func(h searchHit) bool {
			for _, word := range req.toExclude {
				if strings.Contains(h.lowerCasedPath, word) {
					return true
				}
			}
			return false
		})
	}

	if len(hits) == 0 {
		return nil, 0
	}

	res := make([]Hit, 0, len(hits))
	for _, h := range hits {
		path := index.Paths[h.id]
		res = append(res, Hit{
			Path:  path,
			IsDir: strings.HasSuffix(path, "/"),
			Score: h.score,
		})
	}

	slices.SortFunc(res, func(a, b Hit) int {
		return a.compare(b)
	})

	res = compactSearchHits(res)
	total := len(res)

	if len(res) > limit {
		res = res[:limit]
	}

	return res, total
}

// searchByPrefixes searches for prefix matches. If passed "search" contains multiple words,
// only results that match all these words will be returned.
func (index *prefixIndex) searchByPrefixes(words [][]rune) []searchHit {
	var (
		matchCounts        = make(map[uint64]int)
		matchesForAllWords = make(map[uint64]bool)
	)
	for _, word := range words {
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

		// No reason to continue search - all words have to match.
		if len(matches) == 0 {
			return nil
		}

		if len(matchesForAllWords) == 0 {
			// Fill matchesForAllWords on the first match.
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
			if len(matchesForAllWords) == 0 {
				return nil
			}
		}
	}

	hits := make([]searchHit, 0, len(matchesForAllWords))
	for id := range matchesForAllWords {
		hits = append(hits, index.newSearchHit(id, float64(matchCounts[id])))
	}
	return hits
}

func (index *prefixIndex) newSearchHit(id uint64, score float64) searchHit {
	return searchHit{
		id:             id,
		lowerCasedPath: index.lowerCasedPaths[id],
		score:          score,
	}
}

// compactSearchHits merges paths like '/a/b/', '/a/b/c', '/a/b/d/e/' and etc. to just '/a/b/'
// if their scores are equal. Passed hits must be sorted.
func compactSearchHits(hits []Hit) []Hit {
	if len(hits) == 0 {
		return hits
	}

	// Just in case.
	isSorted := slices.IsSortedFunc(hits, func(a, b Hit) int { return a.compare(b) })
	if !isSorted {
		panic("hits must be sorted")
	}

	// There's a very high chance that most of the search hits will be merged into a single
	// entry. So, don't preallocate the resulting slice.
	res := []Hit{
		hits[0],
	}
	for i, hit := range hits {
		if i == 0 {
			continue
		}

		last := res[len(res)-1]
		if hit.Score == last.Score && strings.HasPrefix(hit.Path, last.Path) {
			continue
		}
		res = append(res, hit)
	}
	return res
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

type searchRequest struct {
	words        [][]rune
	exactMatches []string
	toExclude    []string

	extractedWords []string // only for testing
}

func newSearchRequest(search string) (req searchRequest) {
	search = strings.ToLower(search)

	var (
		idx = 0

		// get returns the current character.
		get = func() (byte, bool) {
			if idx < len(search) {
				return search[idx], true
			}
			return 0, false
		}
		// move advances the scanner.
		move = func() {
			idx++
		}
		// readUntil advances the scanner *after* the first occurrence of 'until'
		// and returns all characters, excluding 'until'.
		readUntil = func(until byte) (res string) {
			if _, ok := get(); !ok {
				return ""
			}

			start := idx
			for {
				r, ok := get()
				move()
				if !ok {
					return search[start:]
				}
				if r == until {
					return search[start : idx-1]
				}
			}
		}
	)

	for {
		r, ok := get()
		if !ok {
			break
		}
		if r == ' ' {
			move()
			continue
		}

		var (
			exclude bool
			exact   bool
			until   byte = ' '
		)
		switch r {
		case '"':
			until = '"'
			exact = true
			move()

		case '-':
			exclude = true
			move()
			if r, _ := get(); r == '"' {
				until = '"'
				move()
			}
		}

		word := readUntil(until)
		word = strings.TrimSpace(word)
		if len(word) == 0 {
			continue
		}

		switch {
		case exact:
			req.exactMatches = append(req.exactMatches, word)
		case exclude:
			req.toExclude = append(req.toExclude, word)
		default:
			req.words = append(req.words, splitToNormalizedWords(word)...)

			if testing.Testing() {
				req.extractedWords = append(req.extractedWords, word)
			}
		}
	}
	return req
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

	// Normalize text. For example, 'ü' (U+00FC) is transformed into 2 code
	// points: U+0075 U+0308. We use only letters and digits, so, we will
	// index 'ü' as 'u'.
	v = norm.NFKD.String(v)

	for _, r := range v {
		switch {
		case r == '/' || r == '.' || unicode.IsSpace(r):
			appendWord()
		case '0' <= r && r <= '9':
			word = append(word, r)
		case unicode.IsLetter(r):
			word = append(word, unicode.ToLower(r))
		}
	}
	appendWord()

	return res
}
