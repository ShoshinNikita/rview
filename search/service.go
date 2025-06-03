package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ShoshinNikita/rview/pkg/metrics"
	"github.com/ShoshinNikita/rview/pkg/rlog"
	"github.com/ShoshinNikita/rview/rview"
)

type Service struct {
	rclone Rclone
	cache  Cache

	stopCh    chan struct{}
	stoppedCh chan struct{}

	mu            sync.RWMutex
	indexes       *builtIndexes
	indexesFileID rview.FileID

	minPrefixLen int
	maxPrefixLen int
}

type Rclone interface {
	GetAllFiles(ctx context.Context) (dirs, files []string, err error)
}

type Cache interface {
	Open(id rview.FileID) (io.ReadCloser, error)
	Write(id rview.FileID, r io.Reader) (err error)
}

type builtIndexes struct {
	Index *prefixIndex `json:"index"`

	CreatedAt time.Time `json:"created_at"`
}

func NewService(rclone Rclone, cache Cache) *Service {
	const (
		minPrefixLen = 3
		maxPrefixLen = 10
	)

	return &Service{
		rclone: rclone,
		cache:  cache,
		//
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
		//
		indexesFileID: rview.NewFileID("_prefix_search_indexes.json", 0, 0),
		//
		minPrefixLen: minPrefixLen,
		maxPrefixLen: maxPrefixLen,
	}
}

func (s *Service) Start() (err error) {
	defer func() {
		if err != nil {
			close(s.stoppedCh)
			return
		}

		go s.startBackgroundRefresh()
	}()

	s.indexes, err = s.loadIndexesFromCache()
	if err == nil {
		return nil
	}

	rlog.Infof("couldn't load search indexes from cache, prepare new ones: %s", err)

	// The first few requests can fail with error "connection refused" because
	// rclone is still starting.
	for i := 1; true; i++ {
		err = s.RefreshIndexes(context.Background())
		if err == nil {
			return nil
		}

		err = fmt.Errorf("couldn't prepare search indexes, try %d: %w", i, err)
		if i > 5 {
			return err
		}

		rlog.Debug(err)

		// Exponential Backoff: 100ms -> 200ms -> 400ms -> 800ms -> 1.4s (https://exponentialbackoffcalculator.com)
		time.Sleep(100 * time.Millisecond * time.Duration(math.Pow(1.7, float64(i))))
	}
	panic("unreachable")
}

func (s *Service) loadIndexesFromCache() (res *builtIndexes, err error) {
	rc, err := s.cache.Open(s.indexesFileID)
	if err != nil {
		return nil, fmt.Errorf("cache error: %w", err)
	}
	defer rc.Close()

	err = json.NewDecoder(rc).Decode(&res)
	if err != nil {
		return nil, fmt.Errorf("decode error: %w", err)
	}
	if res == nil || res.Index == nil {
		return nil, errors.New("some indexes are not ready")
	}
	if err := res.Index.Check(s.minPrefixLen, s.maxPrefixLen); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *Service) startBackgroundRefresh() {
	const (
		checkInterval   = time.Hour
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
			createdAt := s.indexes.CreatedAt
			s.mu.RUnlock()

			if time.Since(createdAt) < refreshInterval {
				continue
			}

			err := s.RefreshIndexes(context.Background())
			if err != nil {
				rlog.Errorf("couldn't refresh search indexes: %s", err)
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
	if s.indexes == nil {
		return nil, 0, errors.New("indexes are not ready")
	}

	hits, total = s.indexes.Index.Search(search, limit)
	return hits, total, nil
}

// RefreshIndexes requests all files from rclone and creates new indexes.
func (s *Service) RefreshIndexes(ctx context.Context) (finalErr error) {
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
		rlog.Infof("search indexes were successfully refreshed in %s, dirs: %d, files: %d", dur, dirCount, fileCount)
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
	indexes := &builtIndexes{
		Index:     newPrefixIndex(allEntries, s.minPrefixLen, s.maxPrefixLen),
		CreatedAt: time.Now(),
	}

	// Save indexes to cache before updating in-memory state to avoid
	// any inconsistency.
	err = s.saveIndexesToCache(indexes)
	if err != nil {
		return fmt.Errorf("couldn't save new indexes: %w", err)
	}

	s.mu.Lock()
	s.indexes = indexes
	s.mu.Unlock()

	return nil
}

func (s *Service) saveIndexesToCache(indexes *builtIndexes) error {
	// Don't store encoded indexes in memory because they can be very large.
	r, w := io.Pipe()
	go func() {
		err := json.NewEncoder(w).Encode(indexes)
		if err != nil {
			err = fmt.Errorf("couldn't encode indexes: %w", err)
		}
		w.CloseWithError(err)
	}()

	err := s.cache.Write(s.indexesFileID, r)
	if err != nil {
		return fmt.Errorf("couldn't write indexes to cache: %w", err)
	}
	return nil
}
