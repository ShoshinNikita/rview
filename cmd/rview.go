package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

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

	thumbnailService   ThumbnailService
	thumbnailCache     *cache.DiskCache
	originalImageCache *cache.DiskCache

	searchService *search.Service

	rcloneInstance *rclone.Rclone

	server *web.Server
}

type ThumbnailService interface {
	web.ThumbnailService

	Shutdown(context.Context) error
}

func NewRview(cfg rview.Config) *Rview {
	return &Rview{
		cfg: cfg,
	}
}

func (r *Rview) Prepare() (err error) {
	serviceCache, err := cache.NewDiskCache("rview", filepath.Join(r.cfg.Dir, "rview"), cache.Options{
		// Service cache doesn't need any cleanups.
		DisableCleaner: true,
	})
	if err != nil {
		return fmt.Errorf("couldn't prepare disk cache for service needs: %w", err)
	}

	// Rclone
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

		r.thumbnailCache, err = cache.NewDiskCache(
			"thumbnails", filepath.Join(r.cfg.Dir, "thumbnails"), cache.Options{
				MaxSize: r.cfg.ThumbnailsCacheSize.Bytes(),
			},
		)
		if err != nil {
			return fmt.Errorf("couldn't prepare disk cache for thumbnails: %w", err)
		}

		r.originalImageCache, err = cache.NewDiskCache(
			"original-images", filepath.Join(r.cfg.Dir, "original-images"), cache.Options{
				MaxSize: r.cfg.ThumbnailsOriginalImageCacheSize.Bytes(),
			},
		)
		if err != nil {
			return fmt.Errorf("couldn't prepare disk cache for original images: %w", err)
		}

		r.thumbnailService = thumbnails.NewThumbnailService(
			r.rcloneInstance, r.thumbnailCache, r.originalImageCache, r.cfg.ThumbnailsWorkersCount,
			r.cfg.ThumbnailsFormat,
		)

	} else {
		rlog.Debug("thumbnail service is disabled")

		r.thumbnailService = thumbnails.NewNoopThumbnailService()
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
		"web server":           r.server,
		"thumbnail service":    r.thumbnailService,
		"thumbnail cache":      r.thumbnailCache,
		"original image cache": r.originalImageCache,
		"search service":       r.searchService,
		"rclone instance":      r.rcloneInstance,
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
