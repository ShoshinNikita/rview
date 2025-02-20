package rview

import (
	"context"
	"io"
)

type ThumbnailSize string

const (
	ThumbnailSmall  ThumbnailSize = "small"
	ThumbnailMedium ThumbnailSize = "medium"
	ThumbnailLarge  ThumbnailSize = "large"
)

type ThumbnailService interface {
	CanGenerateThumbnail(FileID) bool
	OpenThumbnail(context.Context, FileID, ThumbnailSize) (rc io.ReadCloser, contentType string, err error)
	Shutdown(context.Context) error
}
