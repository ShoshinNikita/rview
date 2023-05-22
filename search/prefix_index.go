package search

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
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

	lowerCasedPaths map[uint64]string
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

func filterHits(hits []searchHit, f func(searchHit) bool) (res []searchHit) {
	for _, h := range hits {
		if f(h) {
			res = append(res, h)
		}
	}
	return res
}

func (index *prefixIndex) Search(search string, limit int) []rview.SearchHit {
	req := newSearchRequest(search)
	if len(req.words) == 0 && len(req.exactMatches) == 0 && len(req.toExclude) == 0 {
		// Just in case
		return nil
	}

	var hits []searchHit
	if len(req.words) > 0 {
		hits = index.searchByPrefixes(req.words)

	} else {
		// Only exact matches, only excludes, or both - have to check all paths.
		for id := range index.Paths {
			hits = append(hits, index.newSearchHit(id, math.Inf(1)))
		}
	}

	// Filter by exact matches.
	if len(req.exactMatches) > 0 {
		hits = filterHits(hits, func(h searchHit) bool {
			for _, exact := range req.exactMatches {
				if !strings.Contains(h.lowerCasedPath, exact) {
					return false
				}
			}
			return true
		})
	}

	// Filter by excludes.
	if len(req.toExclude) > 0 {
		hits = filterHits(hits, func(h searchHit) bool {
			for _, word := range req.toExclude {
				if strings.Contains(h.lowerCasedPath, word) {
					return false
				}
			}
			return true
		})
	}

	res := make([]rview.SearchHit, 0, len(hits))
	for _, h := range hits {
		res = append(res, rview.SearchHit{
			Path:  index.Paths[h.id],
			Score: h.score,
		})
	}

	sort.Slice(res, func(i, j int) bool {
		a := res[i]
		b := res[j]
		if a.Score == b.Score {
			return a.Path < b.Path
		}
		return a.Score > b.Score
	})

	if len(res) > limit {
		res = res[:limit]
	}

	return res
}

// searchByPrefixes searches for prefix matches. If passed "search" contains multiple words,
// only results that matched all these words will be returned.
func (index *prefixIndex) searchByPrefixes(words [][]rune) []searchHit {
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

	searchForWords string // only for testing
}

var (
	toExcludeRegexp    = regexp.MustCompile(`-"(.+?)"`)
	exactMatchesRegexp = regexp.MustCompile(`"(.+?)"`)
)

func newSearchRequest(search string) (req searchRequest) {
	search = strings.ToLower(search)

	req.toExclude, search = extractSearchTokens(toExcludeRegexp, search)
	req.exactMatches, search = extractSearchTokens(exactMatchesRegexp, search)

	req.searchForWords = search
	req.words = splitToNormalizedWords(search)

	return req
}

func extractSearchTokens(r *regexp.Regexp, search string) (res []string, newSearch string) {
	matches := r.FindAllStringSubmatch(search, -1)
	if len(matches) == 0 {
		return nil, search
	}
	for _, match := range matches {
		if len(match) != 2 {
			panic(fmt.Errorf("invalid number of matches: %v", match))
		}
		res = append(res, match[1])
	}

	newSearch = r.ReplaceAllString(search, "")
	newSearch = strings.TrimSpace(newSearch)

	return res, newSearch
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
