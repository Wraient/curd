package internal

import "testing"

func TestGetLinksFromSourceUrlsPrefersDirectPlayableSources(t *testing.T) {
	t.Parallel()

	sourceUrls := []allanimeSource{
		{
			SourceUrl:  "--175948514e4c4f57",
			SourceName: "Luf-Mp4",
			Priority:   7.5,
		},
		{
			SourceUrl:  "https://tools.fast4speed.rsvp/media9/videos/example/sub/1?Authorization=test",
			SourceName: "Yt-mp4",
			Priority:   7.9,
		},
		{
			SourceUrl:  "https://mp4upload.com/embed-example.html",
			SourceName: "Mp4",
			Priority:   4,
		},
	}

	links, err := getLinksFromSourceUrls(sourceUrls)
	if err != nil {
		t.Fatalf("expected direct Allanime sources to be returned without error, got %v", err)
	}

	if len(links) != 1 {
		t.Fatalf("expected one direct playable source, got %d: %#v", len(links), links)
	}

	if got, want := links[0], "https://tools.fast4speed.rsvp/media9/videos/example/sub/1?Authorization=test"; got != want {
		t.Fatalf("getLinksFromSourceUrls() returned %q, want %q", got, want)
	}
}

func TestSortAllanimeSourcesByPriorityOrdersDescending(t *testing.T) {
	t.Parallel()

	sourceUrls := []allanimeSource{
		{SourceUrl: "https://example.com/low.mp4", Priority: 1},
		{SourceUrl: "https://example.com/high.mp4", Priority: 9},
		{SourceUrl: "https://example.com/mid.mp4", Priority: 5},
	}

	sorted := sortAllanimeSourcesByPriority(sourceUrls)
	if len(sorted) != 3 {
		t.Fatalf("expected 3 sorted sources, got %d", len(sorted))
	}

	if sorted[0].SourceUrl != "https://example.com/high.mp4" {
		t.Fatalf("expected highest priority source first, got %#v", sorted)
	}
	if sorted[1].SourceUrl != "https://example.com/mid.mp4" {
		t.Fatalf("expected middle priority source second, got %#v", sorted)
	}
	if sorted[2].SourceUrl != "https://example.com/low.mp4" {
		t.Fatalf("expected lowest priority source last, got %#v", sorted)
	}
}

func TestIsDirectPlayableAllanimeSource(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		source allanimeSource
		want   bool
	}{
		{
			name: "fast4speed player url",
			source: allanimeSource{
				SourceName: "Yt-mp4",
				SourceUrl:  "https://tools.fast4speed.rsvp/media9/videos/example/sub/1?Authorization=test",
			},
			want: true,
		},
		{
			name: "sharepoint mp4 source",
			source: allanimeSource{
				SourceName: "S-mp4",
				SourceUrl:  "https://tenant.sharepoint.com/video.mp4",
			},
			want: true,
		},
		{
			name: "mp4upload embed url",
			source: allanimeSource{
				SourceName: "Mp4",
				SourceUrl:  "https://mp4upload.com/embed-example.html",
			},
			want: false,
		},
		{
			name: "filemoon embed url",
			source: allanimeSource{
				SourceName: "Fm-Hls",
				SourceUrl:  "https://bysekoze.com/e/example",
			},
			want: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := isDirectPlayableAllanimeSource(tc.source); got != tc.want {
				t.Fatalf("isDirectPlayableAllanimeSource(%+v) = %v, want %v", tc.source, got, tc.want)
			}
		})
	}
}
