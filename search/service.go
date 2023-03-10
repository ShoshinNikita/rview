package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ShoshinNikita/rview/pkg/metrics"
	"github.com/ShoshinNikita/rview/pkg/rlog"
	"github.com/ShoshinNikita/rview/rview"
)

type Service struct {
	rclone Rclone
	cache  rview.Cache

	stopCh    chan struct{}
	stoppedCh chan struct{}

	mu            sync.RWMutex
	indexes       *builtIndexes
	indexesFileID rview.FileID

	minPrefixLen int
	maxPrefixLen int
}

type Rclone interface {
	GetAllFiles(ctx context.Context) ([]string, error)
}

type builtIndexes struct {
	Dirs  *prefixIndex `json:"dirs"`
	Files *prefixIndex `json:"files"`

	CreatedAt time.Time `json:"created_at"`
}

func NewService(rclone Rclone, cache rview.Cache) *Service {
	// TODO: allow to configure?
	const (
		minPrefixLen = 2
		maxPrefixLen = 7
	)

	return &Service{
		rclone: rclone,
		cache:  cache,
		//
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
		//
		indexesFileID: rview.NewFileID("_prefix_search_indexes.json", 0),
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

	if err := s.RefreshIndexes(context.Background()); err != nil {
		return fmt.Errorf("couldn't prepare search indexes: %w", err)
	}
	return nil
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
	if res == nil || res.Dirs == nil || res.Files == nil {
		return nil, errors.New("some indexes are not ready")
	}

	if err := res.Dirs.Check(s.minPrefixLen, s.maxPrefixLen); err != nil {
		return nil, err
	}
	if err := res.Files.Check(s.minPrefixLen, s.maxPrefixLen); err != nil {
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

func (s *Service) Search(ctx context.Context, search string, dirLimit, fileLimit int) (dirs, files []rview.SearchHit, err error) {
	now := time.Now()
	defer func() {
		metrics.SearchDuration.Observe(time.Since(now).Seconds())
	}()

	s.mu.RLock()
	defer s.mu.RUnlock()

	dirs = s.indexes.Dirs.Search(search, dirLimit)
	files = s.indexes.Files.Search(search, fileLimit)

	return dirs, files, nil
}

// RefreshIndexes requests all files from rclone and creates new indexes.
func (s *Service) RefreshIndexes(ctx context.Context) (finalErr error) {
	var (
		now          = time.Now()
		entriesCount int
	)
	defer func() {
		// Monitor duration even for errors.
		dur := time.Since(now)
		metrics.SearchRefreshIndexesDuration.Observe(dur.Seconds())

		if finalErr != nil {
			metrics.SearchRefreshIndexesErrors.Inc()
			return
		}
		rlog.Infof("indexes were successfully refreshed in %s, entries count: %d", dur, entriesCount)
	}()

	allFilenames, err := s.rclone.GetAllFiles(ctx)
	if err != nil {
		return fmt.Errorf("couldn't get all files from rclone: %w", err)
	}
	entriesCount = len(allFilenames)

	var dirs, filenames []string
	for _, f := range allFilenames {
		if strings.HasSuffix(f, "/") {
			dirs = append(dirs, f)
		} else {
			filenames = append(filenames, f)
		}
	}

	indexes := &builtIndexes{
		Dirs:      newPrefixIndex(dirs, s.minPrefixLen, s.maxPrefixLen),
		Files:     newPrefixIndex(filenames, s.minPrefixLen, s.maxPrefixLen),
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
	buf := bytes.NewBuffer(nil)
	err := json.NewEncoder(buf).Encode(indexes)
	if err != nil {
		return fmt.Errorf("couldn't encode indexes: %w", err)
	}

	err = s.cache.Write(s.indexesFileID, buf)
	if err != nil {
		return fmt.Errorf("couldn't write indexes to cache: %w", err)
	}
	return nil
}
