package rview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type Rclone interface {
	OpenFile(ctx context.Context, id FileID) (io.ReadCloser, error)
	GetDirInfo(ctx context.Context, path string, sort, order string) (*RcloneDirInfo, error)
	ProxyFileRequest(id FileID, w http.ResponseWriter, req *http.Request)
}

type RcloneError struct {
	StatusCode int
	BodyPrefix string
}

func (err *RcloneError) Error() string {
	return fmt.Sprintf("unexpected rclone response: status code: %d, body prefix: %q", err.StatusCode, err.BodyPrefix)
}

func IsRcloneNotFoundError(err error) bool {
	var rcloneErr *RcloneError
	return errors.As(err, &rcloneErr) && rcloneErr.StatusCode == http.StatusNotFound
}

type RcloneDirInfo struct {
	Sort  string `json:"sort"`
	Order string `json:"order"`

	Breadcrumbs []RcloneDirBreadcrumb `json:"breadcrumbs"`
	Entries     []RcloneDirEntry      `json:"entries"`
}

type RcloneDirBreadcrumb struct {
	Text string `json:"text"`
}

type RcloneDirEntry struct {
	URL     string `json:"url"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"`
}
