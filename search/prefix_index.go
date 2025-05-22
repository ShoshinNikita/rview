package search

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

type Hit struct {
	Path  string
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
	paths := make(map[uint64]string, len(rawPaths))
	for i, path := range rawPaths {
		paths[uint64(i)] = path //nolint:gosec
	}

	prefixes := make(map[string][]uint64)
	for id, path := range paths {
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

type searchOptions struct {
	compactHits bool
}

func (opts searchOptions) CompactHits() searchOptions {
	opts.compactHits = true
	return opts
}

func (index *prefixIndex) Search(search string, limit int, opts ...func(searchOptions) searchOptions) []Hit {
	searchOpts := searchOptions{
		compactHits: false,
	}
	for _, fn := range opts {
		searchOpts = fn(searchOpts)
	}

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
		return nil
	}

	res := make([]Hit, 0, len(hits))
	for _, h := range hits {
		res = append(res, Hit{
			Path:  index.Paths[h.id],
			Score: h.score,
		})
	}

	slices.SortFunc(res, func(a, b Hit) int {
		return a.compare(b)
	})

	if searchOpts.compactHits {
		res = compactSearchHits(res)
	}

	if len(res) > limit {
		res = res[:limit]
	}

	return res
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

	res := make([]Hit, 0, len(hits))
	res = append(res, hits[0]) // len(res) is always > 0
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
