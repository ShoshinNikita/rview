package search

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ShoshinNikita/rview/pkg/metrics"
	"github.com/ShoshinNikita/rview/pkg/rlog"
)

type Service struct {
	rclone Rclone
	dir    *os.Root

	stopCh    chan struct{}
	stoppedCh chan struct{}

	mu    sync.RWMutex
	index *searchIndex

	minPrefixLen int
	maxPrefixLen int
	filename     string
}

type Rclone interface {
	GetAllFiles(ctx context.Context) (dirs, files []string, err error)
}

type searchIndex struct {
	Index *prefixIndex `json:"index"`

	CreatedAt time.Time `json:"created_at"`
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

	s.index, err = s.loadIndexFromCache()
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

func (s *Service) loadIndexFromCache() (res *searchIndex, err error) {
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
		metrics.SearchDuration.Observe(time.Since(now).Seconds())
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

// RefreshIndex requests all files from rclone and creates a new index.
func (s *Service) RefreshIndex(ctx context.Context) (finalErr error) {
	var (
		now       = time.Now()
		dirCount  int
		fileCount int
	)
	defer func() {
		// Monitor duration even for errors.
		dur := time.Since(now)
		metrics.SearchRefreshIndexesDuration.Observe(dur.Seconds())

		if finalErr != nil {
			metrics.SearchRefreshIndexesErrors.Inc()
			return
		}
		rlog.Infof("search index has been successfully refreshed in %s, dirs: %d, files: %d", dur, dirCount, fileCount)
	}()

	dirs, filenames, err := s.rclone.GetAllFiles(ctx)
	if err != nil {
		return fmt.Errorf("couldn't get all files from rclone: %w", err)
	}
	for i := range dirs {
		if !strings.HasSuffix(dirs[i], "/") {
			dirs[i] += "/"
		}
	}
	dirCount = len(dirs)
	fileCount = len(filenames)

	allEntries := slices.Concat(filenames, dirs)
	index := &searchIndex{
		Index:     newPrefixIndex(allEntries, s.minPrefixLen, s.maxPrefixLen),
		CreatedAt: time.Now(),
	}

	// Save the index on disk before updating in-memory state to avoid
	// any inconsistency.
	err = s.saveIndexToCache(index)
	if err != nil {
		return fmt.Errorf("couldn't save new index: %w", err)
	}

	s.mu.Lock()
	s.index = index
	s.mu.Unlock()

	return nil
}

func (s *Service) saveIndexToCache(index *searchIndex) error {
	// Don't store encoded index in memory because it can be very large.
	r, w := io.Pipe()
	go func() {
		gzipWriter := gzip.NewWriter(w)

		err := json.NewEncoder(gzipWriter).Encode(index)
		if err != nil {
			w.CloseWithError(fmt.Errorf("couldn't encode index: %w", err))
			return
		}
		if err := gzipWriter.Close(); err != nil {
			w.CloseWithError(fmt.Errorf("couldn't close gzip writer: %w", err))
			return
		}
		_ = w.Close()
	}()

	// TODO: write index to tmp file and then rename it (requires go1.25):
	// https://github.com/golang/go/issues/73041

	f, err := s.dir.Create(s.filename)
	if err != nil {
		return fmt.Errorf("couldn't create file: %w", err)
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	if err != nil {
		return fmt.Errorf("couldn't write index to file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("couldn't close file: %w", err)
	}
	return nil
}
