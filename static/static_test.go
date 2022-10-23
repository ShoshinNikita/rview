package static

import (
	"testing"

	"github.com/ShoshinNikita/rview/util/testutil"
)

func TestData(t *testing.T) {
	err := Prepare()
	testutil.NoError(t, err)

	checkIcon := func(m map[string]string) {
		for _, iconName := range m {
			_, ok := fileIconsData.IconDefinitions[iconName]
			if !ok {
				t.Errorf("icon %q is not found", iconName)
			}
		}
	}
	checkIcon(fileIconsData.FolderNames)
	checkIcon(fileIconsData.FileExtensions)
	checkIcon(fileIconsData.FileNames)

	fs := NewFileIconsFS(false)
	for _, iconPath := range fileIconsData.IconDefinitions {
		f, err := fs.Open(iconPath)
		if err != nil {
			t.Errorf("couldn't open icon %q", iconPath)
		}
		f.Close()
	}
}

func TestGetFileIcon(t *testing.T) {
	err := Prepare()
	testutil.NoError(t, err)

	t.Run("files", func(t *testing.T) {
		for filename, wantIconPath := range map[string]string{
			"x.jpeg":       "image.svg",
			"x.png":        "image.svg",
			"x.mp3":        "audio.svg",
			"x.sql":        "database.svg",
			"main_test.go": "go.svg",
			// No custom icons
			"x.js": "file.svg",
			"x.ts": "file.svg",
		} {
			testutil.Equal(t, wantIconPath, GetFileIcon(filename, false))
		}
	})

	t.Run("folders", func(t *testing.T) {
		for filename, wantIconPath := range map[string]string{
			"tests":   "folder-test.svg",
			"src":     "folder-src.svg",
			"scripts": "folder-scripts.svg",
			"data":    "folder-database.svg",
			"Docs":    "folder-docs.svg",
			// No custom icons
			"dir": "folder.svg",
			"ui":  "folder.svg",
		} {
			testutil.Equal(t, wantIconPath, GetFileIcon(filename, true))
		}
	})
}