package rclone

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/ShoshinNikita/rview/pkg/require"
)

func TestDirCache_cleanUp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := require.New(t)

		cache := newDirCache(time.Minute)

		item := cache.Get("/a")
		r.Equal(0, len(item.gobData))
		err := item.StoreLocked(&DirInfo{Dir: "/a"})
		r.NoError(err)

		time.Sleep(50 * time.Second)

		// Item still exists.
		info, err := cache.Get("/a").LoadLocked()
		r.NoError(err)
		r.Equal(info.Dir, "/a")

		item = cache.Get("/b")
		r.Equal(0, len(item.gobData))
		err = item.StoreLocked(&DirInfo{Dir: "/b"})
		r.NoError(err)

		time.Sleep(20 * time.Second)

		cache.cleanUp()

		// Item must be cleaned up.
		info, err = cache.Get("/a").LoadLocked()
		r.NoError(err)
		r.Equal(info, nil)

		// Item still exists.
		info, err = cache.Get("/b").LoadLocked()
		r.NoError(err)
		r.Equal(info.Dir, "/b")
	})
}
