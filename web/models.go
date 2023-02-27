package web

import (
	"net/url"
	"time"

	"github.com/ShoshinNikita/rview/config"
)

// Service response.
type (
	Info struct {
		config.BuildInfo `json:"-"`

		Sort  string `json:"sort"`
		Order string `json:"order"`

		// Dir is the unescaped path of current directory. It is used for page title.
		Dir string `json:"dir"`
		// Breadcrumbs contains info about parent directories.
		Breadcrumbs []Breadcrumb `json:"breadcrumbs"`
		// Entries contains info about files or directories in the current directory.
		Entries []Entry `json:"entries"`

		// dirURL is the url of current directory, only for internal use.
		dirURL *url.URL
	}

	Breadcrumb struct {
		// Link is an escaped link to a directory.
		Link string `json:"link"`
		// Text is an unescaped name of a directory.
		Text string `json:"text"`
	}

	Entry struct {
		// filepath is an rclone filepath, only for internal use.
		filepath string

		// Filename is the unescaped filename of a file.
		Filename             string    `json:"filename"`
		IsDir                bool      `json:"is_dir,omitempty"`
		Size                 int64     `json:"size,omitempty"`
		HumanReadableSize    string    `json:"human_readable_size,omitempty"`
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
		IconName string `json:"icon_name"`
	}
)

// Rclone response.
type (
	RcloneInfo struct {
		Path  string `json:"path"`
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
