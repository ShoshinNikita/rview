package index

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/ShoshinNikita/rview/pkg/metrics"
	"github.com/ShoshinNikita/rview/pkg/misc"
	"github.com/ShoshinNikita/rview/pkg/rlog"
	"github.com/ShoshinNikita/rview/rclone"
	"github.com/ShoshinNikita/rview/rview"
)

type Service struct {
	rclone Rclone
	dir    *os.Root

	stopCh    chan struct{}
	stoppedCh chan struct{}

	mu    sync.RWMutex
	index *entryIndex

	minPrefixLen int
	maxPrefixLen int
	filename     string
}

type Rclone interface {
	GetAllFiles(ctx context.Context) (iter.Seq[rclone.DirEntry], error)
	OpenFile(ctx context.Context, id rview.FileID) (io.ReadCloser, error)
}

type entryIndex struct {
	Index       *prefixIndex           `json:"index"`
	DirMetadata map[string]dirMetadata `json:"dir_metadata"`

	CreatedAt time.Time `json:"created_at"`
}

type dirMetadata struct {
	DefaultSort  string `json:"default_sort"`
	DefaultOrder string `json:"default_order"`
}

func NewService(rclone Rclone, dirRoot *os.Root) (*Service, error) {
	const (
		minPrefixLen = 3
		maxPrefixLen = 10
	)

	err := dirRoot.Mkdir("search", 0700)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("couldn't create 'search' subdirectory: %w", err)
	}
	searchDirRoot, err := dirRoot.OpenRoot("search")
	if err != nil {
		return nil, fmt.Errorf("couldn't open root: %w", err)
	}

	return &Service{
		rclone: rclone,
		dir:    searchDirRoot,
		//
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
		//
		minPrefixLen: minPrefixLen,
		maxPrefixLen: maxPrefixLen,
		filename:     "search_index.json.gz",
	}, nil
}

func (s *Service) Start() (err error) {
	defer func() {
		if err != nil {
			close(s.stoppedCh)
			return
		}

		go s.startBackgroundRefresh()
	}()

	s.index, err = s.loadIndexFromFile()
	if err == nil {
		rlog.Info("search index has been loaded from the file")
		return nil
	}

	rlog.Infof("prepare new index: couldn't load index from the file: %s", err)

	// The first few requests can fail with error "connection refused" because
	// rclone is still starting.
	for i := 1; true; i++ {
		err = s.RefreshIndex(context.Background())
		if err == nil {
			return nil
		}

		err = fmt.Errorf("couldn't prepare search index, try %d: %w", i, err)
		if i > 5 {
			return err
		}

		rlog.Debug(err)

		// Exponential Backoff: 100ms -> 200ms -> 400ms -> 800ms -> 1.4s (https://exponentialbackoffcalculator.com)
		time.Sleep(100 * time.Millisecond * time.Duration(math.Pow(1.7, float64(i))))
	}
	panic("unreachable")
}

func (s *Service) loadIndexFromFile() (res *entryIndex, err error) {
	rc, err := s.dir.Open(s.filename)
	if err != nil {
		return nil, fmt.Errorf("couldn't open file: %w", err)
	}
	defer rc.Close()

	gzipReader, err := gzip.NewReader(rc)
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()

	err = json.NewDecoder(gzipReader).Decode(&res)
	if err != nil {
		return nil, fmt.Errorf("decode error: %w", err)
	}
	if err := gzipReader.Close(); err != nil {
		return nil, fmt.Errorf("couldn't close gzip reader: %w", err)
	}

	if res == nil || res.Index == nil {
		return nil, errors.New("index is not ready")
	}
	if err := res.Index.Check(s.minPrefixLen, s.maxPrefixLen); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *Service) startBackgroundRefresh() {
	const (
		checkInterval   = time.Minute
		refreshInterval = 24 * time.Hour
	)

	defer close(s.stoppedCh)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return

		case <-ticker.C:
			s.mu.RLock()
			createdAt := s.index.CreatedAt
			s.mu.RUnlock()

			if time.Since(createdAt) < refreshInterval {
				continue
			}

			err := s.RefreshIndex(context.Background())
			if err != nil {
				rlog.Errorf("couldn't refresh search index: %s", err)
			}
		}
	}
}

func (s *Service) Shutdown(ctx context.Context) error {
	close(s.stopCh)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.stoppedCh:
		return nil
	}
}

func (s *Service) GetMinSearchLength() int {
	return s.minPrefixLen
}

func (s *Service) Search(_ context.Context, search string, limit int) (hits []Hit, total int, _ error) {
	now := time.Now()
	defer func() {
		metrics.SearchDuration.UpdateDuration(now)
	}()

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Usually happens in integration tests.
	if s.index == nil || s.index.Index == nil {
		return nil, 0, errors.New("index is not ready")
	}

	hits, total = s.index.Index.Search(search, limit)
	return hits, total, nil
}

func (s *Service) GetDefaultSort(dir string) (sort, order string, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.index.DirMetadata[dir]
	return v.DefaultSort, v.DefaultOrder, ok
}

// RefreshIndex requests all files from rclone and creates a new index.
func (s *Service) RefreshIndex(ctx context.Context) (finalErr error) {
	now := time.Now()
	defer func() {
		// Monitor duration even for errors.
		dur := time.Since(now)
		metrics.SearchRefreshIndexesDuration.Update(dur.Seconds())

		if finalErr != nil {
			metrics.SearchRefreshIndexesErrors.Inc()
			return
		}
		rlog.Infof("search index has been successfully refreshed in %s", dur)
	}()

	rawDirEntries, err := s.rclone.GetAllFiles(ctx)
	if err != nil {
		return fmt.Errorf("couldn't get all files from rclone: %w", err)
	}
	dirEntries, dirMeta, err := s.processRawDirEntries(ctx, rawDirEntries)
	if err != nil {
		return fmt.Errorf("couldn't process raw dir entries: %w", err)
	}

	index := &entryIndex{
		Index:       newPrefixIndex(dirEntries, s.minPrefixLen, s.maxPrefixLen),
		DirMetadata: dirMeta,
		CreatedAt:   time.Now(),
	}

	// Save the index on disk before updating in-memory state to avoid
	// any inconsistency.
	err = s.saveIndexToFile(index)
	if err != nil {
		return fmt.Errorf("couldn't save new index: %w", err)
	}

	s.mu.Lock()
	s.index = index
	s.mu.Unlock()

	return nil
}

func (s *Service) processRawDirEntries(ctx context.Context, rawDirEntries iter.Seq[rclone.DirEntry]) (
	iter.Seq[*dirEntry], map[string]dirMetadata, error,
) {
	var (
		metaFiles    []rclone.DirEntry
		entriesByDir = make(map[string][]*dirEntry)
		dirs         = make(map[string]*dirEntry)
	)
	for e := range rawDirEntries {
		if e.Leaf == ".rview.yaml" || e.Leaf == ".rview.yml" {
			metaFiles = append(metaFiles, e)

			// Don't index meta files.
			continue
		}

		var dir string
		if e.IsDir {
			dir = path.Join(e.URL, "..")
		} else {
			dir = path.Dir(e.URL)
		}
		dir = misc.EnsureSuffix(dir, "/")

		entry := &dirEntry{
			Path:    e.URL,
			IsDir:   e.IsDir,
			Size:    e.Size,
			ModTime: e.ModTime,
		}
		entriesByDir[dir] = append(entriesByDir[dir], entry)
		if e.IsDir {
			dirs[e.URL] = entry
		}
	}

	var (
		warningCount = 0
		dirMeta      = make(map[string]dirMetadata)
	)
	for _, e := range metaFiles {
		id := rview.NewFileID(e.URL, e.ModTime, e.Size)
		info, err := s.loadDirMetaInfo(ctx, id)
		if err != nil {
			return nil, nil, err
		}

		dir := path.Dir(e.URL)
		dir = misc.EnsureSuffix(dir, "/")

		switch info.DefaultSort {
		case "":
			// Ignore

		case "namedirfirst_asc", "namedirfirst_desc",
			"size_asc", "size_desc",
			"time_asc", "time_desc":
			defaultSort, defaultOrder, _ := strings.Cut(info.DefaultSort, "_")
			dirMeta[dir] = dirMetadata{
				DefaultSort:  defaultSort,
				DefaultOrder: defaultOrder,
			}

		default:
			info.addWarning(`invalid "default_sort": %q`, info.DefaultSort)
		}

		for pattern, comment := range info.Annotations {
			pattern = strings.TrimPrefix(pattern, "/")
			pattern = strings.TrimSuffix(pattern, "/")

			if strings.Contains(pattern, "/") {
				info.addWarning("invalid pattern %q: pattern can't match files in other directories", pattern)
				continue
			}
			_, err := path.Match(pattern, "")
			if err != nil {
				info.addWarning("invalid pattern %q: %s", pattern, err)
				continue
			}

			if pattern == "." {
				entry := dirs[dir]
				if entry != nil { // just in case
					entry.Annotations = append(entry.Annotations, comment)
				}
				continue
			}

			var matchCount int
			for _, entry := range entriesByDir[dir] {
				matched, _ := path.Match(pattern, path.Base(entry.Path)) // pattern was already validated
				if matched {
					entry.Annotations = append(entry.Annotations, comment)
					matchCount++
				}
			}
			if matchCount == 0 {
				info.addWarning("pattern %q has no matches", pattern)
			}
		}

		warningCount += len(info.warnings)
		switch len(info.warnings) {
		case 0:
			// No warnings.
		case 1:
			rlog.Warnf("attribute file %s has 1 warning: %s", e.URL, info.warnings[0])
		default:
			const sep = "\n  - "
			rlog.Warnf(
				"attribute file %s has %d warnings:%s",
				e.URL, len(info.warnings), sep+strings.Join(info.warnings, sep),
			)
		}
	}

	metrics.SearchMetaFileWarnings.Set(float64(warningCount))

	resIter := func(yield func(*dirEntry) bool) {
		for _, dirEntries := range entriesByDir {
			for _, entry := range dirEntries {
				if !yield(entry) {
					return
				}
			}
		}
	}
	return resIter, dirMeta, nil
}

type dirMetaInfo struct {
	DefaultSort string            `yaml:"default_sort"`
	Annotations map[string]string `yaml:"annotations"`

	warnings []string
}

func (info *dirMetaInfo) addWarning(s string, v ...any) {
	info.warnings = append(info.warnings, fmt.Sprintf(s, v...))
}

func (s *Service) loadDirMetaInfo(ctx context.Context, id rview.FileID) (info dirMetaInfo, err error) {
	rc, err := s.rclone.OpenFile(ctx, id)
	if err != nil {
		return info, fmt.Errorf("couldn't open file %q: %w", id, err)
	}

	var rawInfo map[string]yaml.RawMessage
	if err = yaml.NewDecoder(rc).Decode(&rawInfo); err != nil {
		info.addWarning("couldn't decode file: %s", err)
	}

	var unknownKeys []string
	for k, v := range rawInfo {
		var err error
		switch k {
		case "default_sort":
			err = yaml.Unmarshal(v, &info.DefaultSort)
		case "annotations":
			err = yaml.Unmarshal(v, &info.Annotations)
		default:
			unknownKeys = append(unknownKeys, k)
		}
		if err != nil {
			info.addWarning("couldn't decode %q: %s", k, yaml.FormatError(err, false, false))
		}
	}
	if len(unknownKeys) > 0 {
		info.addWarning("unknown keys: %v", unknownKeys)
	}

	return info, nil
}

func (s *Service) saveIndexToFile(index *entryIndex) error {
	tmpFilename := s.filename + ".tmp"

	f, err := s.dir.Create(tmpFilename)
	if err != nil {
		return fmt.Errorf("couldn't create tmp file: %w", err)
	}
	defer func() {
		_ = f.Close()
		_ = s.dir.Remove(tmpFilename)
	}()

	gzipWriter := gzip.NewWriter(f)

	err = json.NewEncoder(gzipWriter).Encode(index)
	if err != nil {
		return fmt.Errorf("couldn't encode index: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("couldn't close gzip writer: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("couldn't close tmp file: %w", err)
	}

	err = s.dir.Rename(tmpFilename, s.filename)
	if err != nil {
		return fmt.Errorf("couldn't rename tmp file: %w", err)
	}
	return nil
}
