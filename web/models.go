package web

import "time"

// Service response.
type (
	Info struct {
		Sort  string `json:"sort"`
		Order string `json:"order"`

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

		Filename string    `json:"filename,omitempty"`
		IsDir    bool      `json:"is_dir,omitempty"`
		Size     int64     `json:"size,omitempty"`
		ModTime  time.Time `json:"mod_time"`

		// DirURL is an info url for the child directory (not empty only for directories).
		DirURL string `json:"dir_url,omitempty"`
		// OriginalFileURL is an url that should be used to open an original file.
		OriginalFileURL string `json:"original_file_url,omitempty"`
		// ThumbnailURL is an url that should be used to open a thumbnail file (not empty only for images).
		ThumbnailURL string `json:"thumbnail_url,omitempty"`
		// IconURL is an url to an icon. The icon choice is based of filename and file extension.
		IconURL string `json:"icon_url,omitempty"`
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
