package senshi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubtitleInfoFromURL(t *testing.T) {
	rawURL := "https://embed.example/e/abc/?sub.info=https%3A%2F%2Fninstream.com%2Fmanifest.json"
	if got := subtitleInfoFromURL(rawURL); got != "https://ninstream.com/manifest.json" {
		t.Fatalf("unexpected manifest url %q", got)
	}
}

func TestPickSenshiSubtitleTrackPrefersEnglishDefault(t *testing.T) {
	tracks := []senshiSubtitleTrack{
		{Src: "https://cdn.example/jpn.vtt", Label: "JPN"},
		{Src: "https://cdn.example/eng.vtt", Label: "ENG", Default: true},
	}
	if got := pickSenshiSubtitleTrack(tracks); got != "https://cdn.example/eng.vtt" {
		t.Fatalf("unexpected subtitle %q", got)
	}
}

func TestPickSenshiSubtitleTrackSkipsForcedTrackBeforeDefaultDialogue(t *testing.T) {
	tracks := []senshiSubtitleTrack{
		{Src: "https://cdn.example/sub_3_eng.vtt", Label: "Forced (ENG)"},
		{Src: "https://cdn.example/sub_4_eng.vtt", Label: "ENG", Default: true},
		{Src: "https://cdn.example/sub_5_eng.vtt", Label: "SDH (ENG)"},
	}
	if got := pickSenshiSubtitleTrack(tracks); got != "https://cdn.example/sub_4_eng.vtt" {
		t.Fatalf("picked %q instead of the full dialogue track", got)
	}
}

func TestPickSenshiSubtitleTrackAvoidsForcedEnglishFallback(t *testing.T) {
	tracks := []senshiSubtitleTrack{
		{Src: "https://cdn.example/forced.vtt", Label: "Signs & Songs (ENG)"},
		{Src: "https://cdn.example/dialogue.vtt", Label: "English"},
	}
	if got := pickSenshiSubtitleTrack(tracks); got != "https://cdn.example/dialogue.vtt" {
		t.Fatalf("picked %q instead of the full dialogue track", got)
	}
}

func TestSenshiSubtitleManifestURLUsesServerFM(t *testing.T) {
	serverFM := "https://embed.example/e/abc/?sub.info=https://ninstream.com/manifest.json"
	item := embedItem{ServerFM: &serverFM}
	if got := senshiSubtitleManifestURL(item); got != "https://ninstream.com/manifest.json" {
		t.Fatalf("unexpected manifest url %q", got)
	}
}

func TestSenshiSubtitleManifestURLFallsBackToMaskedBase(t *testing.T) {
	item := embedItem{MaskedBaseURL: "https://ninstream.com/example/base"}
	if got := senshiSubtitleManifestURL(item); got != "https://ninstream.com/example/base/sub_filemoon.json" {
		t.Fatalf("unexpected manifest url %q", got)
	}
}

func TestValidatedSenshiASSURLPrefersOriginalStyledTrack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sub_2_eng.ass" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\nFormat: Name\n"))
	}))
	defer server.Close()
	withSenshiTestClient(t, server.Client())

	got := validatedSenshiASSURL(server.URL + "/sub_2_eng.vtt")
	if got != server.URL+"/sub_2_eng.ass" {
		t.Fatalf("validatedSenshiASSURL() = %q", got)
	}
}

func TestSanitizeSenshiWebVTTKeepsMultilineSignInsideCue(t *testing.T) {
	raw := []byte("WEBVTT\n\n05:14.750 --> 05:20.800\n<b>Attack\n\n\nDefense\n \nMagic\n\n\nSpeed</b>\n\n05:21.530 --> 05:22.800\nElymas.\n")
	sanitized, changed := sanitizeSenshiWebVTT(raw)
	if !changed {
		t.Fatal("expected malformed VTT to be changed")
	}
	text := string(sanitized)
	if strings.Contains(text, "\n\nDefense") {
		t.Fatalf("blank line still terminates the cue:\n%s", text)
	}
	if !strings.Contains(text, "Defense") || !strings.Contains(text, "05:21.530 --> 05:22.800\nElymas.") {
		t.Fatalf("sanitized cues were lost:\n%s", text)
	}
}

func TestSanitizeSenshiWebVTTReplacesASSEscapedSpaces(t *testing.T) {
	raw := []byte("WEBVTT\n\n04:59.000 --> 05:01.000\nEpisode 2\\h\\hTitle\n")
	sanitized, changed := sanitizeSenshiWebVTT(raw)
	if !changed || strings.Contains(string(sanitized), "\\h") {
		t.Fatalf("ASS spacing was not normalized: %q", sanitized)
	}
}
