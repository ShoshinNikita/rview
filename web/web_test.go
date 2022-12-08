package web

import (
	"errors"
	"testing"
	"time"

	"github.com/ShoshinNikita/rview/cache"
	"github.com/ShoshinNikita/rview/pkg/util/testutil"
	"github.com/ShoshinNikita/rview/resizer"
	"github.com/ShoshinNikita/rview/rview"
)

func TestServer_sendResizeImageTasks(t *testing.T) {
	t.Parallel()

	stub := &imageResizerStub{
		ImageResizer: resizer.NewImageResizer(cache.NewNoopCache(), 0),
	}
	s := Server{
		resizer: stub,
	}

	zeroModTime := time.Unix(0, 0)

	gotInfo := s.sendResizeImageTasks(Info{
		Entries: []Entry{
			{filepath: "a.txt", ModTime: zeroModTime},
			{filepath: "b.jpg", ModTime: zeroModTime},
			{filepath: "c.png", ModTime: zeroModTime},
			{filepath: "c.bmp", ModTime: zeroModTime},
			{filepath: "d.zip", ModTime: zeroModTime},
			{filepath: "error.jpg", ModTime: zeroModTime},
			{filepath: "resized.jpg", ModTime: zeroModTime},
		},
	})
	testutil.Equal(t, 3, stub.resizeCount)

	testutil.Equal(t,
		Info{Entries: []Entry{
			{filepath: "a.txt", ModTime: zeroModTime}, // no thumbnail: text file
			{filepath: "b.jpg", ModTime: zeroModTime, ThumbnailURL: "/api/thumbnail?filepath=b.jpg&mod_time=0"},
			{filepath: "c.png", ModTime: zeroModTime, ThumbnailURL: "/api/thumbnail?filepath=c.png&mod_time=0"},
			{filepath: "c.bmp", ModTime: zeroModTime},     // no thumbnail: unsupported image
			{filepath: "d.zip", ModTime: zeroModTime},     // no thumbnail: archive
			{filepath: "error.jpg", ModTime: zeroModTime}, // no thumbnail: got error
			{filepath: "resized.jpg", ModTime: zeroModTime, ThumbnailURL: "/api/thumbnail?filepath=resized.jpg&mod_time=0"},
		}},
		gotInfo,
	)

}

type imageResizerStub struct {
	rview.ImageResizer

	resizeCount int
}

func (s *imageResizerStub) IsResized(id rview.FileID) bool {
	return id.GetName() == "resized.jpg"
}

func (s *imageResizerStub) Resize(id rview.FileID, openFileFn rview.OpenFileFn) error {
	s.resizeCount++

	if id.GetName() == "error.jpg" {
		return errors.New("error")
	}
	return nil
}
