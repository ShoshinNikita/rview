// Package metrics provides access to Prometheus metrics.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "rview"

// Web
var (
	HTTPResponseStatuses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "web",
			Name:      "http_response_statuses_total",
		},
		[]string{"status"},
	)
	HTTPResponseTime = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "web",
			Name:      "http_response_time_seconds",
			Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 15, 30},
		},
		[]string{"path"},
	)
)

// Rclone
var (
	RcloneResponseTime = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "rclone",
			Name:      "response_time_seconds",
			Buckets:   []float64{0.05, 0.1, 0.2, 0.5, 1, 2, 5},
		},
	)
)

// Thumbnails
var (
	ThumbnailsErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "thumbnails",
			Name:      "errors_total",
		},
	)
	ThumbnailsOriginalImageUsed = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "thumbnails",
			Name:      "original_image_used",
		},
	)
	ThumbnailsOriginalImageSizes = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "thumbnails",
			Name:      "original_image_size_bytes",
			Buckets: []float64{
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
		},
	)
	ThumbnailsProcessTaskDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "thumbnails",
			Name:      "process_task_duration_seconds",
			Buckets:   []float64{0.2, 0.5, 1, 2, 5, 10, 15, 30, 45, 60, 90, 120},
		},
	)
	ThumbnailsSizeRatio = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "thumbnails",
			Name:      "size_ratio",
			Buckets:   []float64{0.7, 0.9, 1, 2, 5, 10, 20, 30, 50, 70, 100, 150},
		},
	)
)

// Search
var (
	SearchDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "search",
			Name:      "duration_seconds",
			Buckets: []float64{
				0.001, // 1ms
				0.002, // 2ms
				0.005, // 5ms
				0.01,  // 10ms
				0.02,  // 20ms
			},
		},
	)
	SearchRefreshIndexesErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "search",
			Name:      "refresh_indexes_errors_total",
		},
	)
	SearchRefreshIndexesDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "search",
			Name:      "refresh_indexes_duration_seconds",
			Buckets: []float64{
				0.5, // 500ms
				1,   // 1s
				2,   // 2s
				5,   // 5s
				10,  // 10s
				20,  // 20s
				30,  // 30s
			},
		},
	)
)

// Cache
var (
	CacheHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "cache",
			Name:      "hits_total",
		},
	)
	CacheMisses = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "cache",
			Name:      "misses_total",
		},
	)
	CacheErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "cache",
			Name:      "errors_total",
		},
	)
)

// Init values for common labels.
func init() {
	for _, status := range []string{"200", "400", "404", "500"} {
		HTTPResponseStatuses.With(prometheus.Labels{"status": status}).Add(0)
	}
}
