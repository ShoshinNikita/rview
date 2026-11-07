package rview

import (
	"encoding/json"
	"testing"

	"github.com/ShoshinNikita/rview/pkg/require"
)

var testFileID = NewFileID("hello/world.txt", 123456, 1024)

func TestFileID_String(t *testing.T) {
	require.New(t).Equal("t123456_s1024_hello/world.txt", testFileID.String())
}

func TestFileID_JSON(t *testing.T) {
	r := require.New(t)

	var s struct {
		ID FileID `json:"id"`
	}
	s.ID = testFileID

	data, err := json.Marshal(s)
	r.NoError(err)
	r.Equal(`{"id":"t123456_s1024_hello/world.txt"}`, string(data))

	s.ID = FileID{}

	err = json.Unmarshal(data, &s)
	r.NoError(err)
	r.Equal(s.ID.GetModTime(), 123456)
	r.Equal(s.ID.GetSize(), 1024)
	r.Equal(s.ID.GetPath(), "hello/world.txt")
}
