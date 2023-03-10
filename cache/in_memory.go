package cache

import (
	"bytes"
	"errors"
	"io"

	"github.com/ShoshinNikita/rview/rview"
)

type InMemoryCache struct {
	cache map[rview.FileID][]byte
}

func NewInMemoryCache() *InMemoryCache {
	return &InMemoryCache{
		make(map[rview.FileID][]byte),
	}
}

func (c *InMemoryCache) Open(id rview.FileID) (io.ReadCloser, error) {
	data, ok := c.cache[id]
	if !ok {
		return nil, rview.ErrCacheMiss
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (c *InMemoryCache) Check(id rview.FileID) error {
	_, ok := c.cache[id]
	if !ok {
		return rview.ErrCacheMiss
	}
	return nil
}

func (c *InMemoryCache) GetFilepath(rview.FileID) (string, error) {
	return "", errors.New("in-memory cache")
}

func (c *InMemoryCache) Write(id rview.FileID, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	c.cache[id] = data
	return nil
}

func (c *InMemoryCache) Remove(id rview.FileID) error {
	delete(c.cache, id)
	return nil
}
