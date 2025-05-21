package static

import (
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbeddedFileIcons(t *testing.T) {
	allIcons := map[string]struct{}{
		defaultFileIcon:   {},
		defaultFolderIcon: {},
	}
	for _, icon := range fileIconsByFileType {
		allIcons[icon] = struct{}{}
	}
	for ext, icon := range fileIconsByExtension {
		if !strings.HasPrefix(ext, ".") {
			t.Errorf("extension %q must have leading dot", ext)
		}
		allIcons[icon] = struct{}{}
	}

	fs := NewIconsFS(false)
	for icon := range allIcons {
		f, err := fs.Open(path.Join(string(MaterialIconsPack), icon+".svg"))
		if err != nil {
			t.Errorf("couldn't open icon %q", icon)
		} else {
			err = f.Close()
			require.NoError(t, err)
		}
	}
}

func TestGetFileIcon(t *testing.T) {
	t.Run("files", func(t *testing.T) {
		for filename, wantIconPath := range map[string]string{
			"x.jpeg":       "image",
			"x.png":        "image",
			"x.mp3":        "audio",
			"x.mp4":        "video",
			"x.sql":        "document",
			"x.7z":         "zip",
			"x.exe":        "exe",
			"x.pdf":        "pdf",
			"main_test.go": "document",
			"x.js":         "document",
			"x.json":       "json",
			"x.qwerty":     "file",
			"0451":         "file",
		} {
			require.Equal(t, wantIconPath, GetFileIcon(filename, false), filename)
		}
	})

	t.Run("folders", func(t *testing.T) {
		for filename, wantIconPath := range map[string]string{
			"tests":   "folder",
			"src":     "folder",
			"scripts": "folder",
			"data":    "folder",
			"Docs":    "folder",
			"dir":     "folder",
			"ui":      "folder",
		} {
			require.Equal(t, wantIconPath, GetFileIcon(filename, true), filename)
		}
	})
}
