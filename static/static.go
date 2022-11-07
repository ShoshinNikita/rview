package static

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

//go:embed rclone.gotmpl
var RcloneTemplate string

//go:embed templates
var templatesFS embed.FS

func NewTemplatesFS(readFromDisk bool) fs.FS {
	return newFS(templatesFS, "templates", readFromDisk)
}

//go:embed styles
var stylesFS embed.FS

func NewStylesFS(readFromDisk bool) fs.FS {
	return newFS(stylesFS, "styles", readFromDisk)
}

//go:embed js
var scriptsFS embed.FS

func NewScriptsFS(readFromDisk bool) fs.FS {
	return newFS(scriptsFS, "js", readFromDisk)
}

//go:embed feathericons/icons
var iconsFS embed.FS

func NewIconsFS(readFromDisk bool) fs.FS {
	return newFS(iconsFS, "feathericons/icons", readFromDisk)
}

var (
	//go:embed material-icons/icons
	fileIconsFS embed.FS

	//go:embed material-icons/icons.json
	rawFileIconsData []byte
)

func NewFileIconsFS(readFromDisk bool) fs.FS {
	return newFS(fileIconsFS, "material-icons/icons", readFromDisk)
}

type FileIconsData struct {
	ready bool

	IconDefinitions map[string]string `json:"icon_definitions"`
	FolderNames     map[string]string `json:"folder_names"`
	FileExtensions  map[string]string `json:"file_extensions"`
	FileNames       map[string]string `json:"file_names"`
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
