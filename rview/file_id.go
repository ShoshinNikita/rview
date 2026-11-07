package rview

import (
	"bytes"
	"encoding/json/jsontext"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

type FileID struct {
	path    string // full path
	name    string // only filename
	modTime int64  // unix time
	size    int64
}

// NewFileID returns a new [FileID] with cleaned filepath and filename.
func NewFileID(filepath string, modTime int64, size int64) FileID {
	filepath = path.Clean(filepath)
	name := path.Base(filepath)

	return FileID{
		path:    filepath,
		name:    name,
		modTime: modTime,
		size:    size,
	}
}

// GetPath returns full filepath.
func (id FileID) GetPath() string {
	return id.path
}

// GetEscapedPath should be used when building urls with [net/url.JoinPath]
// (see https://github.com/golang/go/issues/75799).
func (id FileID) GetEscapedPath() string {
	var escapedPath strings.Builder
	for part := range strings.SplitSeq(id.path, "/") {
		if part == "" {
			continue
		}
		escapedPath.WriteByte('/')
		escapedPath.WriteString(url.PathEscape(part))
	}
	return escapedPath.String()
}

// GetName returns only filename (last path element).
func (id FileID) GetName() string {
	return id.name
}

// GetExt returns the filename extension in lower case with leading dot (.html).
func (id FileID) GetExt() string {
	return GetFileExt(id.name)
}

func GetFileExt(filepath string) string {
	return strings.ToLower(path.Ext(filepath))
}

// GetModTime returns the modification time.
func (id FileID) GetModTime() int64 {
	return id.modTime
}

// GetSize returns the file size
func (id FileID) GetSize() int64 {
	return id.size
}

func (id FileID) String() string {
	return string(id.appendTo(nil, false))
}

func (id FileID) MarshalText() ([]byte, error) {
	return id.appendTo(nil, false), nil
}

var fileIdRegexp = regexp.MustCompile(`^t(\d+)_s(\d+)_(.+)$`)

func (id *FileID) UnmarshalText(text []byte) error {
	matches := fileIdRegexp.FindSubmatch(text)
	if len(matches) == 0 {
		return fmt.Errorf("invalid file id format: %q", text)
	}

	modTime, err := strconv.ParseInt(string(matches[1]), 10, 64)
	if err != nil {
		return fmt.Errorf("couldn't parse mod time: %w", err)
	}
	size, err := strconv.ParseInt(string(matches[2]), 10, 64)
	if err != nil {
		return fmt.Errorf("couldn't parse size: %w", err)
	}
	filepath := string(matches[3])

	*id = FileID{
		path:    filepath,
		name:    path.Base(filepath),
		modTime: modTime,
		size:    size,
	}
	return nil
}

func (id FileID) MarshalJSONTo(enc *jsontext.Encoder) error {
	text := id.appendTo(enc.AvailableBuffer(), true)
	return enc.WriteValue(text)
}

func (id *FileID) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	text, err := dec.ReadValue()
	if err != nil {
		return err
	}
	text = bytes.TrimPrefix(text, []byte(`"`))
	text = bytes.TrimSuffix(text, []byte(`"`))

	return id.UnmarshalText(text)
}

func (id *FileID) appendTo(b []byte, quote bool) []byte {
	buf := bytes.NewBuffer(b)

	if quote {
		buf.WriteByte('"')
	}
	//
	_ = buf.WriteByte('t')
	_, _ = fmt.Fprint(buf, id.modTime)
	//
	_, _ = buf.WriteString("_s")
	_, _ = fmt.Fprint(buf, id.size)
	//
	_ = buf.WriteByte('_')
	_, _ = buf.WriteString(id.path)
	//
	if quote {
		buf.WriteByte('"')
	}

	return buf.Bytes()
}
