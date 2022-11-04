package web

import (
	"time"

	"github.com/ShoshinNikita/rview/config"
)

// Service response.
type (
	Info struct {
		config.BuildInfo `json:"-"`

		Sort  string `json:"sort"`
		Order string `json:"order"`

		Dir         string       `json:"dir"`
		Breadcrumbs []Breadcrumb `json:"breadcrumbs"`
		Entries     []Entry      `json:"entries"`
	}

	Breadcrumb struct {
		Link string `json:"link"`
		Text string `json:"text"`
	}

	Entry struct {
		// filepath is an rclone filepath, only for internal use.
		filepath string

		Filename             string    `json:"filename,omitempty"`
		IsDir                bool      `json:"is_dir,omitempty"`
		Size                 int64     `json:"size,omitempty"`
		HumanReadableSize    string    `json:"human_readable_size"`
		ModTime              time.Time `json:"mod_time"`
		HumanReadableModTime string    `json:"human_readable_mod_time"`

		// DirURL is an info url for the child directory (not empty only for directories).
		DirURL string `json:"dir_url,omitempty"`
		// WebDirURL is an url to the web page for the child directory (not empty only for directories).
		WebDirURL string `json:"web_dir_url,omitempty"`
		// OriginalFileURL is an url that should be used to open an original file.
		OriginalFileURL string `json:"original_file_url,omitempty"`
		// ThumbnailURL is an url that should be used to open a thumbnail file (not empty only for images).
		ThumbnailURL string `json:"thumbnail_url,omitempty"`
		// IconName is an name of an file icon. The icon choice is based on filename and file extension.
		IconName string `json:"icon_name,omitempty"`
	}
)

// Rclone response.
type (
	RcloneInfo struct {
		Name  string `json:"name"`
		Sort  string `json:"sort"`
		Order string `json:"order"`

		Breadcrumbs []RcloneBreadcrumb `json:"breadcrumbs"`
		Entries     []RcloneEntry      `json:"entries"`
	}

	RcloneBreadcrumb struct {
		Link string `json:"link"`
		Text string `json:"text"`
	}

	RcloneEntry struct {
		URL     string `json:"url"`
		IsDir   bool   `json:"is_dir"`
		Size    int64  `json:"size"`
		ModTime int64  `json:"mod_time"`
	}
)
