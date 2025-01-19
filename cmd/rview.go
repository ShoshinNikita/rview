package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/ShoshinNikita/rview/pkg/cache"
	"github.com/ShoshinNikita/rview/pkg/rlog"
	"github.com/ShoshinNikita/rview/rclone"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/ShoshinNikita/rview/search"
	"github.com/ShoshinNikita/rview/thumbnails"
	"github.com/ShoshinNikita/rview/web"
)

type Rview struct {
	cfg rview.Config

	thumbnailService rview.ThumbnailService
	thumbnailCleaner rview.CacheCleaner

	searchService *search.Service

	rcloneInstance *rclone.Rclone

	server *web.Server
}

func NewRview(cfg rview.Config) *Rview {
	return &Rview{
		cfg: cfg,
	}
}

func (r *Rview) Prepare() (err error) {
	rlog.SetLevel(r.cfg.LogLevel)

	// Note: service cache doesn't need any cleanups.
	serviceCache, err := cache.NewDiskCache(filepath.Join(r.cfg.Dir, "rview"))
	if err != nil {
		return fmt.Errorf("couldn't prepare disk cache for service needs: %w", err)
	}

	// Rclone Instance
	r.rcloneInstance, err = rclone.NewRclone(r.cfg.Rclone)
	if err != nil {
		return fmt.Errorf("couldn't prepare rclone: %w", err)
	}

	// Thumbnail Service
	if r.cfg.ImagePreviewMode == rview.ImagePreviewModeThumbnails {
		err := thumbnails.CheckVips()
		if err != nil {
			return err
		}

		thumbnailsCacheDir := filepath.Join(r.cfg.Dir, "thumbnails")
		thumbnailsCache, err := cache.NewDiskCache(thumbnailsCacheDir)
		if err != nil {
			return fmt.Errorf("couldn't prepare disk cache for thumbnails: %w", err)
		}

		maxFileAge := 24 * time.Hour * time.Duration(r.cfg.ThumbnailsMaxAgeInDays)
		maxTotalFileSize := int64(r.cfg.ThumbnailsMaxTotalSizeInMB * 1 << 20)

		r.thumbnailCleaner = cache.NewCleaner(thumbnailsCacheDir, maxFileAge, maxTotalFileSize)
		r.thumbnailService = thumbnails.NewThumbnailService(
			r.rcloneInstance.GetFile, thumbnailsCache, r.cfg.ThumbnailsWorkersCount, false,
		)

	} else {
		rlog.Debug("thumbnail service is disabled")

		r.thumbnailService = thumbnails.NewNoopThumbnailService()
		r.thumbnailCleaner = cache.NewNoopCleaner()
	}

	// Search Service
	r.searchService = search.NewService(r.rcloneInstance, serviceCache)

	// Web Server
	r.server = web.NewServer(r.cfg, r.rcloneInstance, r.thumbnailService, r.searchService)

	return nil
}

func (r *Rview) Start(onError func()) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		var wg sync.WaitGroup
		for name, s := range map[string]interface{ Start() error }{
			"rclone instance": r.rcloneInstance,
			"search service":  r.searchService,
			"web server":      r.server,
		} {
			name := name
			s := s

			wg.Add(1)
			go func() {
				defer wg.Done()

				if err := s.Start(); err != nil {
					rlog.Errorf("%s error: %s", name, err)
					onError()
				}
			}()
		}
		wg.Wait()

		close(done)
	}()

	return done
}

// Shutdown shutdowns all components. It is safe to call this method even if Prepare has failed.
func (r *Rview) Shutdown(ctx context.Context) error {
	var failed []string
	for name, s := range map[string]shutdowner{
		"web server":              r.server,
		"thumbnail service":       r.thumbnailService,
		"thumbnail cache cleaner": r.thumbnailCleaner,
		"search service":          r.searchService,
		"rclone instance":         r.rcloneInstance,
	} {
		err := safeShutdown(ctx, s)
		if err != nil {
			rlog.Errorf("couldn't shutdown %s gracefully: %s", name, err)

			failed = append(failed, name)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("couldn't gracefully shutdown %s", strings.Join(failed, ", "))
	}
	return nil
}

type shutdowner interface {
	Shutdown(context.Context) error
}

// safeShutdown calls Shutdown method only on initialized components.
func safeShutdown(ctx context.Context, s shutdowner) error {
	v := reflect.ValueOf(s)
	if !v.IsValid() || v.IsNil() {
		return nil
	}
	return s.Shutdown(ctx)
}
