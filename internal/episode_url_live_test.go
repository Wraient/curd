package internal

import (
	"os"
	"strings"
	"testing"
)

func TestAllanimeMarchLionDubEp1Live(t *testing.T) {
	if os.Getenv("CURD_LIVE_ALLANIME_TEST") != "1" {
		t.Skip("set CURD_LIVE_ALLANIME_TEST=1")
	}
	withAllProvidersEnabledForTest(t)

	showID := "NPpCgzb2MbnpiyxZ3"
	provider, err := ProviderByName("allanime")
	if err != nil {
		t.Fatalf("allanime provider: %v", err)
	}
	resolver, ok := provider.(ProviderModeResolver)
	if !ok {
		t.Fatal("expected allanime provider to implement ProviderModeResolver")
	}
	links, err := resolver.GetEpisodeURLForMode(CurdConfig{SubOrDub: "dub"}, showID, 1, "dub")
	if err == nil {
		for _, link := range links {
			if strings.Contains(link, "fast4speed.rsvp") {
				t.Fatalf("dub should not return unreliable fast4speed links, got %s", link)
			}
		}
		t.Fatalf("expected dub lookup to fail without playable AllAnime sources, got links: %#v", links)
	}
}

func TestAllanimeMarchLionSubEp1Live(t *testing.T) {
	if os.Getenv("CURD_LIVE_ALLANIME_TEST") != "1" {
		t.Skip("set CURD_LIVE_ALLANIME_TEST=1")
	}
	withAllProvidersEnabledForTest(t)

	showID := "NPpCgzb2MbnpiyxZ3"
	provider, err := ProviderByName("allanime")
	if err != nil {
		t.Fatalf("allanime provider: %v", err)
	}
	resolver, ok := provider.(ProviderModeResolver)
	if !ok {
		t.Fatal("expected allanime provider to implement ProviderModeResolver")
	}
	links, err := resolver.GetEpisodeURLForMode(CurdConfig{SubOrDub: "sub"}, showID, 1, "sub")
	if err != nil {
		t.Fatalf("sub episode link lookup failed: %v", err)
	}
	if len(links) == 0 {
		t.Fatal("expected sub links")
	}
	picked := PrioritizeLink(links)
	if strings.Contains(picked, "fast4speed.rsvp") {
		t.Fatalf("sub should not play fast4speed when wixstatic providers exist, got %s", picked)
	}
	if !strings.Contains(picked, "video.wixstatic.com") &&
		!strings.Contains(picked, "repackager.wixmp.com") &&
		!strings.Contains(picked, "sharepoint.com") {
		t.Fatalf("unexpected sub link source: %s", picked)
	}
}
