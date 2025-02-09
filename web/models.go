package web

import (
	"net/url"
	"time"

	"github.com/ShoshinNikita/rview/rview"
)

type DirInfo struct {
	rview.BuildInfo `json:"-"`

	Sort  string `json:"sort"`
	Order string `json:"order"`

	// Dir is the unescaped path of the requested directory.
	Dir string `json:"dir"`
	// Breadcrumbs contains info about parent directories.
	Breadcrumbs []DirBreadcrumb `json:"breadcrumbs"`
	// Entries contains info about files or directories in the current directory.
	Entries []DirEntry `json:"entries"`

	DirCount      int   `json:"dir_count"`
	FileCount     int   `json:"file_count"`
	TotalFileSize int64 `json:"total_file_size"`

	// IsNotFound indicates whether the requested directory wasn't found.
	IsNotFound bool `json:"is_not_found"`

	// dirURL is the url of current directory, only for internal use.
	dirURL *url.URL
}

type DirBreadcrumb struct {
	// Link is an escaped link to a directory.
	Link string `json:"link"`
	// Text is an unescaped name of a directory.
	Text string `json:"text"`
}

type DirEntry struct {
	// filepath is an rclone filepath, only for internal use.
	filepath string

	// Filename is the unescaped filename of a file.
	Filename             string         `json:"filename"`
	IsDir                bool           `json:"is_dir,omitempty"`
	Size                 int64          `json:"size,omitempty"`
	HumanReadableSize    string         `json:"human_readable_size,omitempty"`
	ModTime              time.Time      `json:"mod_time"`
	HumanReadableModTime string         `json:"human_readable_mod_time"`
	FileType             rview.FileType `json:"file_type,omitempty"`
	CanPreview           bool           `json:"can_preview"`

	// DirURL is an info url for the child directory (not empty only for directories).
	DirURL string `json:"dir_url,omitempty"`
	// WebDirURL is an url to the web page for the child directory (not empty only for directories).
	WebDirURL string `json:"web_dir_url,omitempty"`
	// OriginalFileURL is an url that should be used to open an original file.
	OriginalFileURL string `json:"original_file_url,omitempty"`
	// ThumbnailURL is an url that should be used to open a thumbnail file (not empty only for images).
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	// IconName is an name of an file icon. The icon choice is based on filename and file extension.
	IconName string `json:"icon_name"`
}

type SearchResponse struct {
	Dirs  []SearchHit `json:"dirs"`
	Files []SearchHit `json:"files"`
}

type SearchHit struct {
	Path   string  `json:"path"`
	Score  float64 `json:"score"`
	WebURL string  `json:"web_url"`
	Icon   string  `json:"icon"`
}
