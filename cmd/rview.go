package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/ShoshinNikita/rview/cache"
	"github.com/ShoshinNikita/rview/config"
	"github.com/ShoshinNikita/rview/pkg/rlog"
	"github.com/ShoshinNikita/rview/rclone"
	"github.com/ShoshinNikita/rview/rview"
	"github.com/ShoshinNikita/rview/static"
	"github.com/ShoshinNikita/rview/thumbnails"
	"github.com/ShoshinNikita/rview/web"
)

type Rview struct {
	cfg config.Config

	thumbnailService rview.ThumbnailService
	thumbnailCleaner rview.CacheCleaner

	rcloneInstance *rclone.Rclone

	server *web.Server
}

func NewRview(cfg config.Config) (r *Rview) {
	return &Rview{
		cfg: cfg,
	}
}

func (r *Rview) Prepare() (err error) {
	if err := static.Prepare(); err != nil {
		return fmt.Errorf("couldn't prepare icons: %w", err)
	}

	// Thumbnail Service
	if r.cfg.Thumbnails {
		err := thumbnails.CheckVips()
		if err != nil {
			return err
		}

		thumbnailsCacheDir := filepath.Join(r.cfg.Dir, "thumbnails")
		thumbnailsCache, err := cache.NewDiskCache(thumbnailsCacheDir)
		if err != nil {
			return fmt.Errorf("couldn't prepare disk cache for thumbnails: %w", err)
		}
		r.thumbnailCleaner = cache.NewCleaner(thumbnailsCacheDir, r.cfg.ThumbnailsMaxAge, r.cfg.ThumbnailsMaxTotalSize)
		r.thumbnailService = thumbnails.NewThumbnailService(thumbnailsCache, r.cfg.ThumbnailsWorkersCount)

	} else {
		rlog.Info("thumbnail service is disabled")

		r.thumbnailService = thumbnails.NewNoopThumbnailService()
		r.thumbnailCleaner = cache.NewNoopCleaner()
	}

	// Rclone Instance
	r.rcloneInstance, err = rclone.NewRclone(r.cfg.RclonePort, r.cfg.RcloneTarget)
	if err != nil {
		return fmt.Errorf("couldn't prepare rclone: %w", err)
	}

	// Web Server
	r.server = web.NewServer(r.cfg, r.thumbnailService)

	return nil
}

func (r *Rview) Start(onError func()) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		var wg sync.WaitGroup
		for name, s := range map[string]interface{ Start() error }{
			"rclone instance": r.rcloneInstance,
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
