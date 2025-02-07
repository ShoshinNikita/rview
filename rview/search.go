package rview

import "context"

type SearchService interface {
	GetMinSearchLength() int
	Search(ctx context.Context, search string, dirLimit, fileLimit int) (dirs, files []SearchHit, err error)
	RefreshIndexes(ctx context.Context) error
}

type SearchHit struct {
	Path  string
	Score float64
}
