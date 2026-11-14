package anikoto

import (
	"bytes"
	"encoding/json"
)

type ajaxHTMLResponse struct {
	Status int    `json:"status"`
	Result string `json:"result"`
}

type ajaxStatusResponse interface {
	statusCode() int
}

func (r *ajaxHTMLResponse) statusCode() int {
	return r.Status
}

func (r *ajaxURLResponse) statusCode() int {
	return r.Status
}

type ajaxURLResponse struct {
	Status int `json:"status"`
	Result struct {
		URL string `json:"url"`
	} `json:"result"`
}

type episodeMetadata struct {
	Number int
	IDs    string
	Sub    bool
	Dub    bool
}

type serverCandidate struct {
	Type string
	ID   string
	Name string
}

type resolvedStream struct {
	URL      string
	Referrer string
	Subtitle string
}

type megaPlaySourcesResponse struct {
	Enc     string          `json:"enc"`
	Sources flexibleString  `json:"sources"`
	Tracks  []megaPlayTrack `json:"tracks"`
}

type megaPlayTrack struct {
	File    string `json:"file"`
	Label   string `json:"label"`
	Kind    string `json:"kind"`
	Default bool   `json:"default"`
}

type flexibleString string

func (s *flexibleString) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*s = ""
		return nil
	}
	if data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*s = flexibleString(value)
		return nil
	}
	if data[0] == '[' {
		var values []json.RawMessage
		if err := json.Unmarshal(data, &values); err != nil {
			return err
		}
		if len(values) > 0 {
			return s.UnmarshalJSON(values[0])
		}
		*s = ""
		return nil
	}
	var object struct {
		File string `json:"file"`
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	*s = flexibleString(object.File)
	return nil
}
