package allanime

import (
	"strings"
	"testing"
)

func TestExpandWixmpLinksMatchesAniCLI(t *testing.T) {
	t.Parallel()

	input := "https://repackager.wixmp.com/video.wixstatic.com/video/4fa677_7daec0ccb9364ac7b032d43692c73617/,720p,480p,1080p,/mp4/file.mp4.urlset/master.m3u8"
	links := expandWixmpLinks(input)
	if len(links) != 3 {
		t.Fatalf("expected 3 expanded links, got %d: %#v", len(links), links)
	}
	if !strings.HasPrefix(links[0], "https://video.wixstatic.com/video/4fa677_7daec0ccb9364ac7b032d43692c73617/1080p/mp4/file.mp4") {
		t.Fatalf("expected 1080p wixstatic link first, got %q", links[0])
	}
}

func TestGetLinksFromEncodedSourceUrlsSkipsUnreliableFast4speed(t *testing.T) {
	t.Parallel()

	sourceUrls := []allanimeSource{
		{
			SourceUrl:  "https://tools.fast4speed.rsvp/media9/videos/example/dub/1?Authorization=test",
			SourceName: "Yt-mp4",
			Priority:   7.9,
		},
	}

	_, _, err := getLinksFromEncodedSourceUrls(sourceUrls)
	if err == nil {
		t.Fatal("expected error when only direct fast4speed source is available")
	}
}

func TestGetLinksFromEncodedSourceUrlsRequiresEncodedProviders(t *testing.T) {
	t.Parallel()

	sourceUrls := []allanimeSource{
		{
			SourceUrl:  "https://tenant.sharepoint.com/video.mp4",
			SourceName: "S-mp4",
			Priority:   7.4,
		},
	}

	_, _, err := getLinksFromEncodedSourceUrls(sourceUrls)
	if err == nil {
		t.Fatal("expected error when no encoded provider sources exist")
	}
}

func TestParseAllanimeStreamInfScore(t *testing.T) {
	t.Parallel()

	line := `#EXT-X-STREAM-INF:BANDWIDTH=2800000,RESOLUTION=1280x720,CODECS="avc1.64001f,mp4a.40.2"`
	if got := parseAllanimeStreamInfScore(line); got != 1280 {
		t.Fatalf("parseAllanimeStreamInfScore() = %d, want 1280", got)
	}
}

func TestResolveAllanimeRelativeURL(t *testing.T) {
	t.Parallel()

	got := resolveAllanimeRelativeURL("https://example.com/path/", "720p/index.m3u8")
	want := "https://example.com/path/720p/index.m3u8"
	if got != want {
		t.Fatalf("resolveAllanimeRelativeURL() = %q, want %q", got, want)
	}
}
