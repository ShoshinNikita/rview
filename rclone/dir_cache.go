package rclone

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sync"
	"time"
)

type dirCache struct {
	ttl time.Duration

	mu    sync.Mutex
	cache map[string]*dirCacheItem
}

func newDirCache(ttl time.Duration) *dirCache {
	return &dirCache{
		ttl:   ttl,
		cache: make(map[string]*dirCacheItem),
	}
}

func (c *dirCache) Enabled() bool {
	return c.ttl > 0
}

func (c *dirCache) Get(path string) *dirCacheItem {
	c.mu.Lock()
	defer c.mu.Unlock()

	res, ok := c.cache[path]
	if !ok {
		res = &dirCacheItem{
			ttl: c.ttl,
		}
		c.cache[path] = res
	}
	return res
}

type dirCacheItem struct {
	ttl time.Duration

	mu        sync.Mutex
	gobData   []byte
	expiresAt time.Time
}

func (c *dirCacheItem) Lock() {
	c.mu.Lock()
}

func (c *dirCacheItem) Unlock() {
	c.mu.Unlock()
}

// LoadLocked tries to load [*DirInfo] from the cache. It requires that mutex remains locked
// for the duration of the call.
func (c *dirCacheItem) LoadLocked() (*DirInfo, error) {
	if len(c.gobData) == 0 {
		return nil, nil
	}
	if time.Now().After(c.expiresAt) {
		return nil, nil
	}

	var info DirInfo
	err := gob.NewDecoder(bytes.NewReader(c.gobData)).Decode(&info)
	if err != nil {
		return nil, fmt.Errorf("gob decode failed: %w", err)
	}
	return &info, nil
}

// StoreLocked saves [*DirInfo] to the cache. It requires that mutex remains locked
// for the duration of the call.
func (c *dirCacheItem) StoreLocked(info *DirInfo) error {
	buf := bytes.NewBuffer(nil)
	err := gob.NewEncoder(buf).Encode(info)
	if err != nil {
		return fmt.Errorf("gob encode failed: %w", err)
	}
	c.gobData = buf.Bytes()
	c.expiresAt = time.Now().Add(c.ttl)

	return nil
}
