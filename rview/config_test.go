package rview

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImagePreviewMode(t *testing.T) {
	r := require.New(t)

	var v ImagePreviewMode
	r.Error(v.UnmarshalText([]byte("xxx")))

	r.NoError(v.UnmarshalText([]byte("original")))
	r.Equal(ImagePreviewModeOriginal, v)
}

func TestThumbnailsFormat(t *testing.T) {
	r := require.New(t)

	var v ThumbnailsFormat
	r.Error(v.UnmarshalText([]byte("xxx")))

	r.NoError(v.UnmarshalText([]byte("jpeg")))
	r.Equal(JpegThumbnails, v)
}

func TestMiB(t *testing.T) {
	for _, tt := range []struct {
		in        string
		wantErr   string
		wantText  string
		wantBytes int64
	}{
		{in: "1Mi", wantText: "1Mi", wantBytes: 1 << 20},
		{in: "500Mi", wantText: "500Mi", wantBytes: 500 << 20},
		{in: "1024Mi", wantText: "1Gi", wantBytes: 1 << 30},
		{in: "2047Mi", wantText: "2047Mi", wantBytes: 2047 << 20},
		{in: "2048Mi", wantText: "2Gi", wantBytes: 2 << 30},
		{in: "1Gi", wantText: "1Gi", wantBytes: 1 << 30},
		{in: "3Gi", wantText: "3Gi", wantBytes: 3 << 30},
		//
		{in: "3GiB", wantErr: "valid suffixes: Mi, Gi", wantText: "0Mi"},
		{in: "3xGi", wantErr: "invalid size: strconv.Atoi", wantText: "0Mi"},
	} {
		t.Run("", func(t *testing.T) {
			r := require.New(t)

			var s MiB
			err := s.UnmarshalText([]byte(tt.in))
			if tt.wantErr == "" {
				r.NoError(err)
			} else {
				r.Error(err)
				r.Contains(err.Error(), tt.wantErr)
			}

			r.Equal(tt.wantText, s.String())
			r.Equal(tt.wantBytes, s.Bytes())
		})
	}
}
