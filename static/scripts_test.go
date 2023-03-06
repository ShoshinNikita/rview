package static

import (
	"encoding/json"
	"io/fs"
	"os"
	pkgPath "path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ShoshinNikita/rview/pkg/rlog"
	"github.com/stretchr/testify/require"
)

// TestPrepareFileIcons converts `material-icons.json` to a more convenient format and removes
// unnecessary icons. Preparation steps:
//
//  1. Clone https://github.com/PKief/vscode-material-icon-theme
//  2. Run `npm i && npm run build`
//  3. Copy `dist/material-icons.json` and `icons/*`
//  4. Run this script (remove `t.SkipNow`)
func TestPrepareFileIcons(t *testing.T) {
	t.Skip("script")

	r := require.New(t)

	f, err := os.Open("material-icons/material-icons.json")
	r.NoError(err)
	defer f.Close()

	var old OriginalIconsData
	err = json.NewDecoder(f).Decode(&old)
	r.NoError(err)

	new := FileIconsData{
		IconDefinitions: make(map[string]string),
		FolderNames:     old.FolderNames,
		FileExtensions:  old.FileExtensions,
		FileNames:       old.FileNames,
	}
	var openIconsCount, lightIconsCount int
	for iconName, iconPath := range old.IconDefinitions {
		if strings.HasSuffix(iconName, "-open") {
			openIconsCount++
			continue
		}
		if strings.HasSuffix(iconName, "_light") {
			lightIconsCount++
			continue
		}
		new.IconDefinitions[iconName] = pkgPath.Base(iconPath.IconPath)
	}

	f, err = os.Create("material-icons/icons.json")
	r.NoError(err)
	defer f.Close()

	err = json.NewEncoder(f).Encode(new)
	r.NoError(err)

	var removedIconsCount int
	err = filepath.Walk("material-icons/icons", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		var found bool
		for _, fileName := range new.IconDefinitions {
			if fileName == info.Name() {
				found = true
				break
			}
		}
		if found {
			return nil
		}

		removedIconsCount++

		err = os.Remove(path)
		if err != nil {
			t.Logf("couldn't remove icon %q", path)
			t.Fail()
		}
		return nil
	})
	r.NoError(err)

	rlog.Infof(`%d "*-open" icons were removed`, openIconsCount)
	rlog.Infof(`%d "*_light" icons were removed`, lightIconsCount)
	rlog.Infof("%d icons were removed", removedIconsCount)
}

//nolint:tagliatelle
type OriginalIconsData struct {
	IconDefinitions map[string]struct {
		IconPath string `json:"iconPath"`
	} `json:"iconDefinitions"`
	FolderNames    map[string]string `json:"folderNames"`
	FileExtensions map[string]string `json:"fileExtensions"`
	FileNames      map[string]string `json:"fileNames"`
}
