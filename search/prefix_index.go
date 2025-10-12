package search

import (
	"cmp"
	"encoding/json"
	"fmt"
	"iter"
	"math"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/ShoshinNikita/rview/rclone"
	"golang.org/x/text/unicode/norm"
)

type Hit struct {
	Path    string
	IsDir   bool
	Size    int64
	ModTime int64

	Score float32
}

func (h Hit) GetPath() string { return h.Path }
func (h Hit) GetIsDir() bool  { return h.IsDir }

type prefixIndex struct {
	MinPrefixLen int                 `json:"min_prefix_len"`
	MaxPrefixLen int                 `json:"max_prefix_len"`
	Entries      map[uint32]dirEntry `json:"entries"`
	Prefixes     map[string][]uint32 `json:"prefixes"`

	lowerCasedPaths map[uint32]string
}

type dirEntry struct {
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"`
}

func newPrefixIndex(dirEntries iter.Seq[rclone.DirEntry], minPrefixLen, maxPrefixLen int) *prefixIndex {
	var (
		entries  = make(map[uint32]dirEntry)
		prefixes = make(map[string][]uint32)
	)
	var id uint32
	for entry := range dirEntries {
		entries[id] = dirEntry{
			Path:    entry.URL,
			IsDir:   entry.IsDir,
			Size:    entry.Size,
			ModTime: entry.ModTime,
		}
		for prefix := range generatePrefixes(entry.URL, minPrefixLen, maxPrefixLen) {
			prefixes[prefix] = append(prefixes[prefix], id)
		}

		id++
	}
	for prefix, ids := range prefixes {
		slices.Sort(ids)
		prefixes[prefix] = slices.Clone(slices.Compact(ids)) // use slices.Clone because we don't need extra capacity
	}

	index := &prefixIndex{
		MinPrefixLen: minPrefixLen,
		MaxPrefixLen: maxPrefixLen,
		Entries:      entries,
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
	index.lowerCasedPaths = make(map[uint32]string)
	for id, path := range index.Entries {
		index.lowerCasedPaths[id] = strings.ToLower(path.Path)
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
	id             uint32
	score          float32
	lowerCasedPath string
}

func (index *prefixIndex) Search(search string, limit int) ([]Hit, int) {
	req := newSearchRequest(search, index.MinPrefixLen)
	if len(req.words) == 0 && len(req.exactMatches) == 0 && len(req.toExclude) == 0 {
		return nil, 0
	}

	var hitsIter iter.Seq[searchHit]
	if len(req.words) > 0 {
		hitsIter = index.searchByPrefixes(req.words)

	} else {
		// Only exact matches, only excludes, or both - have to check all paths.
		hitsIter = func(yield func(searchHit) bool) {
			for id := range index.Entries {
				if !yield(index.newSearchHit(id, float32(math.Inf(1)))) {
					return
				}
			}
		}
	}

	// Filter by exact matches.
	if len(req.exactMatches) > 0 {
		hitsIter = deleteIter(hitsIter, func(h searchHit) bool {
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
		hitsIter = deleteIter(hitsIter, func(h searchHit) bool {
			for _, word := range req.toExclude {
				if strings.Contains(h.lowerCasedPath, word) {
					return true
				}
			}
			return false
		})
	}

	var res []Hit
	for h := range hitsIter {
		entry := index.Entries[h.id]
		res = append(res, Hit{
			Path:    entry.Path,
			IsDir:   entry.IsDir,
			Size:    entry.Size,
			ModTime: entry.ModTime,
			Score:   h.score,
		})
	}
	if len(res) == 0 {
		return nil, 0
	}

	res = compactSearchHits(res)
	total := len(res)

	slices.SortFunc(res, func(a, b Hit) int {
		if v := cmp.Compare(a.Score, b.Score); v != 0 {
			return -1 * v
		}
		return rclone.CompareDirEntryByName(a, b)
	})

	if len(res) > limit {
		res = res[:limit]
	}

	return res, total
}

// searchByPrefixes checks every word for prefix matches.
func (index *prefixIndex) searchByPrefixes(words [][]rune) iter.Seq[searchHit] {
	noopIter := func(yield func(searchHit) bool) {}

	var (
		matchCounts        = make(map[uint32]int)
		matchesForAllWords = make(map[uint32]bool)
	)
	for _, word := range words {
		// If a word length is less than MinPrefixLen, no prefixes will be generated, and
		// no hits will be returned. So, ignore such words.
		if len(word) < index.MinPrefixLen {
			continue
		}

		matches := make(map[uint32]bool)
		for prefix := range generatePrefixes(string(word), index.MinPrefixLen, index.MaxPrefixLen) {
			for _, id := range index.Prefixes[prefix] {
				matchCounts[id]++

				matches[id] = true
			}
		}

		// No reason to continue search - all words have to match.
		if len(matches) == 0 {
			return noopIter
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
				return noopIter
			}
		}
	}

	return func(yield func(searchHit) bool) {
		for id := range matchesForAllWords {
			if !yield(index.newSearchHit(id, float32(matchCounts[id]))) {
				return
			}
		}
	}
}

func (index *prefixIndex) newSearchHit(id uint32, score float32) searchHit {
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

	slices.SortFunc(hits, func(a, b Hit) int {
		if v := cmp.Compare(a.Score, b.Score); v != 0 {
			return -1 * v
		}

		return cmp.Compare(a.Path, b.Path)
	})

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
		if last.IsDir && hit.Score == last.Score && strings.HasPrefix(hit.Path, last.Path) {
			continue
		}
		res = append(res, hit)
	}
	return res
}

func generatePrefixes(path string, minLen, maxLen int) iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, word := range splitToNormalizedWords(path, minLen) {
			for i := minLen; i <= maxLen; i++ {
				if i > len(word) {
					break
				}
				if !yield(string(word[:i])) {
					return
				}
			}
		}
	}
}

type searchRequest struct {
	words        [][]rune
	exactMatches []string
	toExclude    []string

	extractedWords []string // only for testing
}

func newSearchRequest(search string, minWordLen int) (req searchRequest) {
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
			req.words = append(req.words, splitToNormalizedWords(word, minWordLen)...)

			if testing.Testing() {
				req.extractedWords = append(req.extractedWords, word)
			}
		}
	}
	return req
}

func splitToNormalizedWords(v string, minLen int) (res [][]rune) {
	var (
		word       []rune
		appendWord = func() {
			if len(word) >= minLen {
				res = append(res, word)
				word = nil
			} else {
				word = word[:0] // can reuse slice
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

func deleteIter[T any](seq iter.Seq[T], f func(T) bool) iter.Seq[T] {
	return func(yield func(T) bool) {
		for v := range seq {
			if !f(v) {
				if !yield(v) {
					return
				}
			}
		}
	}
}
