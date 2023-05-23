// Package static provides access to static assets and Go templates for UI rendering.
package static

import (
	"embed"
	"io/fs"

	"github.com/ShoshinNikita/rview/rview"
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

//go:embed material-icons/icons
var fileIconsFS embed.FS

func NewFileIconsFS(readFromDisk bool) fs.FS {
	return newFS(fileIconsFS, "material-icons/icons", readFromDisk)
}

const (
	defaultFileIcon   = "file"
	defaultFolderIcon = "folder"
)

var (
	fileIconsByFileType = map[rview.FileType]string{
		rview.FileTypeImage: "image",
		rview.FileTypeAudio: "audio",
		rview.FileTypeVideo: "video",
		rview.FileTypeText:  "document",
	}

	extensionsByFileIcons = map[string][]string{
		"console":    {".sh", ".zsh", ".bash", ".bat", ".cmd"},
		"disc":       {".iso"},
		"exe":        {".exe", ".msi"},
		"json":       {".json", ".jsonc", ".json5"},
		"pdf":        {".pdf"},
		"powerpoint": {".odp", ".potm", ".potx", ".ppa", ".ppam", ".pps", ".ppsm", ".ppsx", ".ppt", ".pptm", ".pptx"},
		"table":      {".csv", ".ods", ".psv", ".tsv", ".xls", ".xlsm", ".xlsx"},
		"word":       {".doc", ".docx", ".odt", ".rtf"},
		"xml":        {".xml"},
		"yaml":       {".yml", ".yaml"},
		"zip":        {".7z", ".gz", ".gzip", ".rar", ".tar", ".tgz", ".tz", ".zip"},
	}

	fileIconsByExtension map[string]string
)

func init() {
	fileIconsByExtension = make(map[string]string)
	for icon, exts := range extensionsByFileIcons {
		for _, ext := range exts {
			fileIconsByExtension[ext] = icon
		}
	}
}

func GetFileIcon(filename string, isDir bool) string {
	if isDir {
		return defaultFolderIcon
	}

	fileID := rview.NewFileID(filename, 0)

	// Icons by extensions have higher priority.
	icon, ok := fileIconsByExtension[fileID.GetExt()]
	if ok {
		return icon
	}

	icon, ok = fileIconsByFileType[rview.GetFileType(fileID)]
	if ok {
		return icon
	}

	return defaultFileIcon
}
