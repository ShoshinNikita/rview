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

// Image Resizer
var (
	ResizerErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "resizer",
			Name:      "errors_total",
		},
	)
	ResizerOriginalImageUsed = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "resizer",
			Name:      "original_image_used",
		},
	)
	ResizerDownloadedImageSizes = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "resizer",
			Name:      "downloaded_image_size_bytes",
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
	ResizerProcessDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "resizer",
			Name:      "process_duration_seconds",
			Buckets:   []float64{0.2, 0.5, 1, 2, 5, 10, 15, 30, 45, 60, 90, 120},
		},
	)
	ResizerSizeRatio = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "resizer",
			Name:      "size_ratio",
			Buckets:   []float64{0.7, 0.9, 1, 2, 5, 10, 20, 30, 50, 70, 100, 150},
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
