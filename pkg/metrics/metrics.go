// Package metrics provides access to Prometheus metrics.
package metrics

import (
	"fmt"

	"github.com/VictoriaMetrics/metrics"
)

// Web
var (
	HTTPResponseStatuses = func(status int) *metrics.Counter {
		return metrics.GetOrCreateCounter(fmt.Sprintf(`rview_web_http_response_statuses_total{status="%d"}`, status))
	}
	HTTPResponseTime = func(path string) *metrics.PrometheusHistogram {
		return metrics.GetOrCreatePrometheusHistogramExt(
			fmt.Sprintf(`rview_web_http_response_time_seconds{path=%q}`, path),
			[]float64{0.1, 0.5, 1, 2, 5, 10, 15, 30},
		)
	}
)

// Rclone
var (
	RcloneGetDirInfoDuration = metrics.NewPrometheusHistogramExt(
		"rview_rclone_get_dir_info_duration_seconds",
		[]float64{0.05, 0.1, 0.2, 0.5, 1, 2, 5},
	)
	RcloneGetFileHeadersDuration = metrics.NewPrometheusHistogramExt(
		"rview_rclone_get_file_headers_duration_seconds",
		[]float64{0.05, 0.1, 0.2, 0.5, 1, 2, 5},
	)
	RcloneDirsServedFromCache = metrics.NewCounter(
		"rview_rclone_dirs_served_from_cache",
	)
)

// Thumbnails
var (
	ThumbnailsErrors             = metrics.NewCounter("rview_thumbnails_errors_total")
	ThumbnailsOriginalImageUsed  = metrics.NewCounter("rview_thumbnails_original_image_used")
	ThumbnailsOriginalImageSizes = metrics.NewPrometheusHistogramExt(
		"rview_thumbnails_original_image_size_bytes",
		[]float64{
			124 << 10, // 124 Kib
			256 << 10, // 256 Kib
			512 << 10, // 512 Kib
			1 << 20,   // 1 Mib
			2 << 20,   // 2 Mib
			5 << 20,   // 5 Mib
			10 << 20,  // 10 Mib
			15 << 20,  // 15 Mib
			20 << 20,  // 20 Mib
			30 << 20,  // 30 Mib
		},
	)
	ThumbnailsDownloadImageDuration = metrics.NewPrometheusHistogramExt(
		"rview_thumbnails_download_image_duration_seconds",
		[]float64{0.1, 0.2, 0.35, 0.5, 1, 2, 3.5, 5},
	)
	ThumbnailsResizeDuration = metrics.NewPrometheusHistogramExt(
		"rview_thumbnails_resize_duration_seconds",
		[]float64{0.1, 0.2, 0.35, 0.5, 1, 2, 3.5, 5},
	)
	ThumbnailsProcessTaskDuration = metrics.NewPrometheusHistogramExt(
		"rview_thumbnails_process_task_duration_seconds",
		[]float64{0.2, 0.5, 1, 2, 5, 10, 15, 30, 45, 60, 90, 120},
	)
	ThumbnailsSizeRatio = func(size string) *metrics.PrometheusHistogram {
		return metrics.GetOrCreatePrometheusHistogramExt(
			fmt.Sprintf(`rview_thumbnails_size_ratio{thumbnail_size=%q}`, size),
			[]float64{0.7, 0.9, 1, 2, 5, 10, 20, 30, 50, 70, 100, 150},
		)
	}
	ThumbnailsOriginalImagesUsedFromCache = metrics.NewCounter(
		"rview_thumbnails_original_images_used_from_cache",
	)
)

// Search
var (
	SearchDuration = metrics.NewPrometheusHistogramExt(
		"rview_search_duration_seconds",
		[]float64{
			0.001, // 1ms
			0.002, // 2ms
			0.005, // 5ms
			0.01,  // 10ms
			0.02,  // 20ms
		},
	)
	SearchRefreshIndexesErrors = metrics.NewCounter(
		"rview_search_refresh_indexes_errors_total",
	)
	SearchRefreshIndexesDuration = metrics.NewPrometheusHistogramExt(
		"rview_search_refresh_indexes_duration_seconds",
		[]float64{
			0.5, // 500ms
			1,   // 1s
			2,   // 2s
			5,   // 5s
			10,  // 10s
			20,  // 20s
			30,  // 30s
		},
	)
	SearchMetaFileWarnings = metrics.NewGauge("rview_search_meta_file_warnings", nil)
)

// Cache
var (
	CacheHits   = metrics.NewCounter("rview_cache_hits_total")
	CacheMisses = metrics.NewCounter("rview_cache_misses_total")
	CacheErrors = metrics.NewCounter("rview_cache_errors_total")
	CacheSize   = func(name string) *metrics.Gauge {
		return metrics.GetOrCreateGauge(fmt.Sprintf("rview_cache_size_bytes{name=%q}", name), nil)
	}
	CacheCleanerErrors = metrics.NewCounter("rview_cache_cleaner_errors_total")
)

// Init values for common labels.
func init() {
	for _, status := range []int{200, 400, 404, 500} {
		HTTPResponseStatuses(status).Add(0)
	}
}
