package anipub

// FuzzDecodeSearchResults feeds arbitrary bytes to the search-response
// decoder. Provider payloads are untrusted: the decoder must never panic.
//
//	go test ./internal/providers/anipub -run '^$' -fuzz FuzzDecodeSearchResults -fuzztime 30s

import "testing"

func FuzzDecodeSearchResults(f *testing.F) {
	for _, seed := range []string{
		`[{"Name":"Frieren","Id":2454}]`,
		`{"Name":"Dungeon","Id":2475,"finder":"dungeon"}`,
		`{"found":false}`,
		``, `[]`, `{}`, `{not json`, "<html>cloudflare</html>", "\x00\x01",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		results, err := decodeSearchResults([]byte(raw))
		if err == nil {
			for _, result := range results {
				_ = result // shape is enforced by callers; panic is the failure
			}
		}
	})
}
