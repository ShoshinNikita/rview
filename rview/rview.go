// Package rview contains basic models and interfaces.
package rview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

type FileID struct {
	path    string // full path
	name    string // only filename
	modTime int64  // unix time
}

// NewFileID returns a new [FileID] with cleaned filepath and filename.
func NewFileID(filepath string, modTime int64) FileID {
	filepath = path.Clean(filepath)
	name := path.Base(filepath)

	return FileID{
		path:    filepath,
		name:    name,
		modTime: modTime,
	}
}

// GetPath returns full filepath.
func (id FileID) GetPath() string {
	return id.path
}

// GetName returns only filename (last path element).
func (id FileID) GetName() string {
	return id.name
}

// GetExt returns the filename extension in lower case with leading dot (.html).
func (id FileID) GetExt() string {
	return strings.ToLower(path.Ext(id.name))
}

// GetModTime returns the modification time.
func (id FileID) GetModTime() time.Time {
	return time.Unix(id.modTime, 0).UTC()
}

func (id FileID) String() string {
	return fmt.Sprintf("%d_%s", id.modTime, id.path)
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

type Rclone interface {
	GetFile(ctx context.Context, id FileID) (io.ReadCloser, http.Header, error)
	GetDirInfo(ctx context.Context, path string, sort, order string) (*RcloneDirInfo, error)
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

var (
	ErrCacheMiss = errors.New("cache miss")
)

type Cache interface {
	Open(id FileID) (io.ReadCloser, error)
	Check(id FileID) error
	GetFilepath(id FileID) (path string, err error)
	Write(id FileID, r io.Reader) (err error)
	Remove(id FileID) error
}

type CacheCleaner interface {
	Shutdown(ctx context.Context) error
}

type ThumbnailID struct {
	FileID
}

type ThumbnailService interface {
	NewThumbnailID(FileID) ThumbnailID
	CanGenerateThumbnail(FileID) bool
	IsThumbnailReady(ThumbnailID) bool
	OpenThumbnail(context.Context, ThumbnailID) (io.ReadCloser, error)
	SendTask(id FileID) error
	Shutdown(context.Context) error
}

type OpenFileFn func(ctx context.Context, id FileID) (io.ReadCloser, error)

type SearchService interface {
	GetMinSearchLength() int
	Search(ctx context.Context, search string, dirLimit, fileLimit int) (dirs, files []SearchHit, err error)
	RefreshIndexes(ctx context.Context) error
}

type SearchHit struct {
	Path  string
	Score float64
}
