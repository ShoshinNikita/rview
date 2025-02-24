// Package static provides access to static assets and Go templates for UI rendering.
package static

import (
	"embed"
	"io/fs"

	"github.com/ShoshinNikita/rview/rview"
)

//go:embed templates
var templatesFS embed.FS

func NewTemplatesFS(readFromDisk bool) fs.FS {
	return newFS(templatesFS, "templates", readFromDisk)
}

//go:embed css
var stylesFS embed.FS

//nolint:stylecheck,revive
func NewCssFS(readFromDisk bool) fs.FS {
	return newFS(stylesFS, "css", readFromDisk)
}

//go:embed js
var scriptsFS embed.FS

func NewScriptsFS(readFromDisk bool) fs.FS {
	return newFS(scriptsFS, "js", readFromDisk)
}

//go:embed icons
var iconsFS embed.FS

func NewIconsFS(readFromDisk bool) fs.FS {
	return newFS(iconsFS, "icons", readFromDisk)
}

//go:embed pwa
var pwaFS embed.FS

func NewPwaFS(readFromDisk bool) fs.FS {
	return newFS(pwaFS, "pwa", readFromDisk)
}

type IconPack string

const (
	MaterialIconsPack IconPack = "material-icons"
	FeatherIconsPack  IconPack = "feathericons"
)

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

	ext := rview.GetFileExt(filename)

	// Icons by extensions have higher priority.
	icon, ok := fileIconsByExtension[ext]
	if ok {
		return icon
	}

	icon, ok = fileIconsByFileType[rview.GetFileType(ext)]
	if ok {
		return icon
	}

	return defaultFileIcon
}
