// Package mkv provides a convenience interface to mkvtoolnix command line tools.
//
// This package only parses the minimum fields necessary for eac3to-wrapper.
package mkv

import (
	"encoding/json"
	"fmt"
)

// Info represent a info for a mkv file. It is populated by parsing the output of
// `mkvmerge -J file.mkv`.
type Info struct {
	Version int `json:"identification_format_version"`
	Errors  []string
	Tracks  []*Track
}

type TrackProperty struct {
	Number int
}

// A Track represents a track in a MKV segment.
type Track struct {
	Id    int    // the track Id property, not necessarily related to its physical id.
	Type_ string `json:"type"` // "video"/"audio"/"subtitles"

	TrackProperty `json:"properties"`
}

// TrackType represents the type of a track: video/audio or subtitle.
type TrackType int8

const (
	TrackTypeUnknown TrackType = iota
	TrackTypeVideo
	TrackTypeAudio
	TrackTypeSubtitle
)

func (t *Track) Type() TrackType {
	switch t.Type_ {
	case "video":
		return TrackTypeVideo
	case "audio":
		return TrackTypeAudio
	case "subtitles":
		return TrackTypeSubtitle
	}
	return TrackTypeUnknown
}

func ParseInfo(b []byte) (*Info, error) {
	var info Info
	err := json.Unmarshal(b, &info)
	if err != nil {
		return nil, err
	}
	if len(info.Errors) > 0 {
		return &info, fmt.Errorf("mkvmerge error: %q", info.Errors)
	}
	return &info, nil
}
