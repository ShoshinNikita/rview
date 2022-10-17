package cache

import (
	"bytes"
	"io"
	"sync"

	"github.com/ShoshinNikita/rview/rview"
)

type MemoryCache struct {
	mu    sync.RWMutex
	cache map[rview.FileID]*[]byte
}

var _ rview.Cache = (*MemoryCache)(nil)

func NewMemoryCache() *MemoryCache {
	return &MemoryCache{}
}

func (c *MemoryCache) Open(id rview.FileID) (io.ReadCloser, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, ok := c.cache[id]
	if !ok {
		return nil, rview.ErrCacheMiss
	}
	return readCloser{bytes.NewReader(*data)}, nil
}

func (c *MemoryCache) Check(id rview.FileID) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if _, ok := c.cache[id]; !ok {
		return rview.ErrCacheMiss
	}
	return nil
}

func (c *MemoryCache) GetSaveWriter(id rview.FileID) (io.WriteCloser, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	data := new([]byte)
	c.cache[id] = data
	return &writeCloser{data}, nil
}

type readCloser struct {
	io.Reader
}

func (readCloser) Close() error {
	return nil
}

type writeCloser struct {
	dst *[]byte
}

func (wc *writeCloser) Write(data []byte) (int, error) {
	*wc.dst = append(*wc.dst, data...)
	return len(data), nil
}

func (wc *writeCloser) Close() error {
	return nil
}
