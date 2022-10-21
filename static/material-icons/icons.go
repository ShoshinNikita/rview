package icons

import (
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

var (
	//go:embed icons.json
	rawIconsData []byte

	//go:embed icons/*
	IconsFS embed.FS
)

type IconsData struct {
	ready bool

	IconDefinitions map[string]string `json:"iconDefinitions"`
	FolderNames     map[string]string `json:"folderNames"`
	FileExtensions  map[string]string `json:"fileExtensions"`
	FileNames       map[string]string `json:"fileNames"`
}

var iconsData IconsData

func Prepare() error {
	err := json.Unmarshal(rawIconsData, &iconsData)
	if err != nil {
		return fmt.Errorf("couldn't unmarshal material icons info: %w", err)
	}
	iconsData.ready = true

	return nil
}

const (
	defaultFileIconName   = "file"
	defaultFolderIconName = "folder"
)

func GetIconFilename(filename string, isDir bool) string {
	if !iconsData.ready {
		panic("icons are not prepared")
	}

	filename = path.Base(filename)
	if isDir {
		iconName, ok := iconsData.FolderNames[filename]
		if !ok {
			iconName = defaultFolderIconName
		}
		return iconsData.IconDefinitions[iconName]
	}

	iconName, ok := iconsData.FileNames[filename]
	if !ok {
		ext := strings.TrimPrefix(path.Ext(filename), ".")
		iconName, ok = iconsData.FileExtensions[ext]
		if !ok {
			iconName = defaultFileIconName
		}
	}
	return iconsData.IconDefinitions[iconName]
}
