package resizer

import "image"

// thumbnail calculates new width and height preserving original aspect ratio.
// If the current width and height are less than the max ones, it will return
// shouldResize = false.
//
// This function is based on [github.com/nfnt/resize.Thumbnail].
func thumbnail(bounds image.Rectangle, maxWidth, maxHeight int) (newWidth, newHeight int, shouldResize bool) {
	origWidth := bounds.Dx()
	origHeight := bounds.Dy()

	newWidth, newHeight = origWidth, origHeight

	// Resizing is not required.
	if maxWidth >= origWidth && maxHeight >= origHeight {
		return 0, 0, false
	}

	// Preserve aspect ratio.
	switch {
	case origWidth > maxWidth:
		newHeight = origHeight * maxWidth / origWidth
		if newHeight < 1 {
			newHeight = 1
		}
		newWidth = maxWidth

	case newHeight > maxHeight:
		newWidth = newWidth * maxHeight / newHeight
		if newWidth < 1 {
			newWidth = 1
		}
		newHeight = maxHeight
	}

	return newWidth, newHeight, true
}
