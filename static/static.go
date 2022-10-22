package static

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

//go:embed templates
var templatesFS embed.FS

func NewTemplatesFS(debug bool) fs.FS {
	return newFS(templatesFS, "templates", debug)
}

//go:embed styles
var stylesFS embed.FS

func NewStylesFS(debug bool) fs.FS {
	return newFS(stylesFS, "styles", debug)
}

//go:embed feathericons/icons
var iconsFS embed.FS

func NewIconsFS(debug bool) fs.FS {
	return newFS(iconsFS, "feathericons/icons", debug)
}

var (
	//go:embed material-icons/icons
	fileIconsFS embed.FS

	//go:embed material-icons/icons.json
	rawFileIconsData []byte
)

func NewFileIconsFS(debug bool) fs.FS {
	return newFS(fileIconsFS, "material-icons/icons", debug)
}

type FileIconsData struct {
	ready bool

	IconDefinitions map[string]string `json:"iconDefinitions"`
	FolderNames     map[string]string `json:"folderNames"`
	FileExtensions  map[string]string `json:"fileExtensions"`
	FileNames       map[string]string `json:"fileNames"`
}

var fileIconsData FileIconsData

func Prepare() error {
	err := json.Unmarshal(rawFileIconsData, &fileIconsData)
	if err != nil {
		return fmt.Errorf("couldn't unmarshal material icons info: %w", err)
	}
	fileIconsData.ready = true

	return nil
}

func GetFileIcon(filename string, isDir bool) string {
	const (
		defaultFileIconName   = "file"
		defaultFolderIconName = "folder"
	)

	if !fileIconsData.ready {
		panic("icons are not prepared")
	}

	filename = strings.ToLower(path.Base(filename))
	if isDir {
		iconName, ok := fileIconsData.FolderNames[filename]
		if !ok {
			iconName = defaultFolderIconName
		}
		return fileIconsData.IconDefinitions[iconName]
	}

	iconName, ok := fileIconsData.FileNames[filename]
	if !ok {
		ext := strings.TrimPrefix(path.Ext(filename), ".")
		iconName, ok = fileIconsData.FileExtensions[ext]
		if !ok {
			iconName = defaultFileIconName
		}
	}
	return fileIconsData.IconDefinitions[iconName]
}
