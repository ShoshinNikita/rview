package resizer

import (
	"image"
	"testing"

	"github.com/ShoshinNikita/rview/util/testutil"
)

func TestThumbnail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		bounds    image.Rectangle
		maxWidth  int
		maxHeight int
		//
		wantWidth        int
		wantHeight       int
		wantShouldResize bool
	}{
		{
			bounds:    image.Rectangle{Max: image.Point{X: 1000, Y: 800}},
			maxWidth:  1000,
			maxHeight: 1000,
			//
			wantShouldResize: false,
		},
		{
			bounds:    image.Rectangle{Max: image.Point{X: 1100, Y: 800}},
			maxWidth:  1000,
			maxHeight: 1000,
			//
			wantShouldResize: true,
			wantWidth:        1000,
			wantHeight:       727,
		},
		{
			bounds:    image.Rectangle{Max: image.Point{X: 1000, Y: 1400}},
			maxWidth:  1000,
			maxHeight: 500,
			//
			wantShouldResize: true,
			wantWidth:        357,
			wantHeight:       500,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run("", func(t *testing.T) {
			gotWidth, gotHeight, shouldResize := thumbnail(tt.bounds, tt.maxWidth, tt.maxHeight)
			testutil.Equal(t, tt.wantWidth, gotWidth)
			testutil.Equal(t, tt.wantHeight, gotHeight)
			testutil.Equal(t, tt.wantShouldResize, shouldResize)
		})
	}
}
