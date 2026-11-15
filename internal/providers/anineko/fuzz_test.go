package anineko

// FuzzExtractLangEmbedURLs feeds arbitrary HTML to the watch-page scraper.
// Provider pages are untrusted: extraction must never panic and must only
// return the known language groups.
//
//	go test ./internal/providers/anineko -run '^$' -fuzz FuzzExtractLangEmbedURLs -fuzztime 30s

import "testing"

func FuzzExtractLangEmbedURLs(f *testing.F) {
	for _, seed := range []string{
		`<div class="server-items" data-id="sub"><div data-video="https://bibiemb.xyz/aaa"></div></div>`,
		``, `<html></html>`, `data-id="dub" data-video="x"`, "\x00\x01", `data-id="sub"`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, html string) {
		groups := extractLangEmbedURLs(html)
		for lang, urls := range groups {
			switch lang {
			case "hsub", "sub", "dub":
			default:
				t.Fatalf("unexpected language group %q", lang)
			}
			for _, u := range urls {
				if u == "" {
					t.Fatal("empty embed URL returned")
				}
				_ = resolveEmbedHost(u)
				_ = subtitleFromEmbedURL(u)
			}
		}
		_ = slugFromWatchURL(html)
	})
}
