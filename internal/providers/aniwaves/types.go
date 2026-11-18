package aniwaves

import "encoding/json"

// SearchItem is stored in SelectionOption.ExtraData for mapping hints.
type SearchItem struct {
	Slug          string
	Title         string
	JapaneseTitle string
	Type          string
}

// ajaxEnvelope decodes aniwaves ajax responses whose result is either an
// object (search) or a raw HTML string (episode/server lists).
type ajaxEnvelope struct {
	Status int             `json:"status"`
	Result json.RawMessage `json:"result"`
}

type searchPayload struct {
	HTML string `json:"html"`
}

type sourcesResult struct {
	URL    string `json:"url"`
	Server int    `json:"server"`
}

type vhSources struct {
	Sources string `json:"sources"`
	Error   string `json:"error"`
}
