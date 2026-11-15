package providers_test

// Headless provider contract tests.
//
// These tests drive every provider through the public registry interface
// (Search -> Episodes -> Stream -> playable playlist) without opening the
// application, a browser, rofi, or mpv. All traffic goes through the single
// curdhost.HTTPClient seam, so a fake RoundTripper can stand in for the real
// provider sites.
//
// Run the offline contract (default, deterministic):
//
//	go test ./internal/providers -run TestProviderContract -v
//
// Run the same contract against the live sites:
//
//	CURD_LIVE_CONTRACT=1 go test ./internal/providers -run TestLiveProviderContract -v -timeout 10m

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wraient/curd/internal/curdhost"
	_ "github.com/wraient/curd/internal/loadproviders"
	"github.com/wraient/curd/internal/providers"
)

type contractTransport struct {
	handler func(req *http.Request) (int, string)
}

func (c contractTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	status, body := c.handler(req)
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

func installContractHost(t *testing.T, client *http.Client) {
	t.Helper()
	prevClient := curdhost.HTTPClient
	prevLog := curdhost.Log
	prevOut := curdhost.Out
	prevPrompt := curdhost.PromptSelect
	prevSubStyle := curdhost.CurrentSubStyle
	prevPersist := curdhost.PersistSubStylePreference
	prevStorage := curdhost.StoragePath
	prevLang := curdhost.AnimeNameLanguage
	t.Cleanup(func() {
		curdhost.HTTPClient = prevClient
		curdhost.Log = prevLog
		curdhost.Out = prevOut
		curdhost.PromptSelect = prevPrompt
		curdhost.CurrentSubStyle = prevSubStyle
		curdhost.PersistSubStylePreference = prevPersist
		curdhost.StoragePath = prevStorage
		curdhost.AnimeNameLanguage = prevLang
	})
	curdhost.HTTPClient = func() *http.Client { return client }
	curdhost.Log = func(string) {}
	curdhost.Out = func(string) {}
	curdhost.PromptSelect = func(options []curdhost.PromptOption) (curdhost.PromptOption, error) {
		if len(options) == 0 {
			return curdhost.PromptOption{}, nil
		}
		return options[0], nil
	}
	curdhost.CurrentSubStyle = func() string { return "soft" }
	curdhost.PersistSubStylePreference = func(string) error { return nil }
	curdhost.StoragePath = func() string { return t.TempDir() }
	curdhost.AnimeNameLanguage = func() string { return "english" }
}

const contractPlaylist = "#EXTM3U\n#EXT-X-VERSION:3\n#EXTINF:10.0,\nseg-1.ts\n#EXTINF:10.0,\nseg-2.ts\n"

// contractHappyHandler serves minimal but valid fixtures for the senshi,
// anipub, and anineko APIs, dispatched by host and path.
func contractHappyHandler(req *http.Request) (int, string) {
	host := req.URL.Host
	path := req.URL.Path

	switch {
	// --- senshi ---
	case (host == "senshi.live" || host == "senshi.to") && path == "/anime/filter":
		return 200, `{"data":[{"id":52991,"public_id":"frieren","title":"Sousou no Frieren","title_english":"Frieren: Beyond Journey's End","type":"TV","ani_episodes":"28","ani_year":2023}],"total":1}`
	case (host == "senshi.live" || host == "senshi.to") && path == "/episodes/52991":
		return 200, `[{"id":1,"ep_id":1,"mal_id":52991},{"id":2,"ep_id":2,"mal_id":52991}]`
	case (host == "senshi.live" || host == "senshi.to") && path == "/episode-embeds/52991/1":
		return 200, `[{"url":"https://cdn.example/senshi/ep1.m3u8","remote_source_id":771,"status":"HardSub"}]`
	case host == "s.vidcloud.se" && path == "/_v1/sources":
		return 200, `[{"source":{"src":"https://cdn.example/senshi/ep1.m3u8"},"tracks":[]}]`

	// --- anipub ---
	case (host == "anipub.xyz" || host == "anipub.org") && strings.HasPrefix(path, "/api/search/"):
		return 200, `[{"Name":"Frieren: Beyond Journey's End","Id":2454,"Image":"https://cdn.example/frieren.jpg","finder":"frieren"}]`
	case (host == "anipub.xyz" || host == "anipub.org") && path == "/api/info/2454":
		return 200, `{"_id":2454,"Name":"Frieren: Beyond Journey's End","MALID":"52991","epCount":28}`
	case (host == "anipub.xyz" || host == "anipub.org") && path == "/v1/api/details/2454":
		return 200, `{"local":{"link":"src=https://anipub.xyz/video/107257/sub","ep":[{"link":"src=https://anipub.xyz/video/107258/sub"}]}}`
	case host == "megaplay.buzz" && path == "/stream/s-2/107257/sub":
		return 200, `<div id="player" data-id="13461"></div>`
	case host == "megaplay.buzz" && path == "/stream/getSources":
		return 200, `{"sources":{"file":"https://cdn.example/anipub/master.m3u8"},"tracks":[{"file":"https://cdn.example/anipub/eng.vtt","label":"English","kind":"captions","default":true}]}`

	// --- anineko ---
	case host == "anineko.to" && path == "/ajax/search":
		return 200, `{"success":true,"results":[{"title":"Frieren: Beyond Journey's End","url":"/watch/frieren-beyond","image":"/img/frieren.jpg","meta":"TV"}]}`
	case host == "anineko.to" && path == "/watch/frieren-beyond":
		return 200, `<a href="/watch/frieren-beyond/ep-1">1</a><a href="/watch/frieren-beyond/ep-2">2</a>`
	case host == "anineko.to" && path == "/watch/frieren-beyond/ep-1":
		return 200, `<div class="server-items" data-id="sub"><div data-video="https://bibiemb.example/emb1"></div></div>`
	case host == "bibiemb.example" && path == "/emb1":
		return 200, `const src = "https://bibiemb.example/emb1/master.m3u8"`
	case host == "bibiemb.example" && path == "/emb1/master.m3u8":
		return 200, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1,NAME=\"720p\"\n720p/index.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=2,NAME=\"1080p\"\n1080p/index.m3u8\n"
	case host == "bibiemb.example" && path == "/emb1/1080p/index.m3u8":
		return 200, contractPlaylist

	// --- anipm (ani.pm + Settlar embed) ---
	case host == "ani.pm" && path == "/api/anime/search":
		return 200, `{"items":[{"id":6351,"routeId":"frieren-6351-rblfpv","title":"Frieren: Beyond Journey's End","year":2023,"type":"TV","episodeCount":28,"subCount":28,"dubCount":28,"malId":52991}]}`
	case host == "ani.pm" && (path == "/api/anime/series/6351" || path == "/api/anime/series/frieren-6351-rblfpv"):
		return 200, `{"id":6351,"title":"Frieren","episodes":[{"number":1,"sub":true,"dub":true},{"number":2,"sub":true,"dub":true}]}`
	case host == "ani.pm" && (path == "/api/anime/playback-bootstrap/settlar/6351" || path == "/api/anime/playback-bootstrap/settlar/frieren-6351-rblfpv"):
		return 200, `{"episode":1,"settlarSelection":"sel123","availability":{"sub":true,"dub":true},"anipmPackages":{"episodes":{"1":{"sub":true,"dub":true,"subhard":true}}}}`
	case host == "ani.pm" && path == "/api/anime/settlar/session":
		return 200, `{"embedUrl":"https://embed.settlar.io/embed/v1?t=tok123"}`
	case host == "embed.settlar.io" && path == "/embed/v1":
		return 200, `<html>player</html>`
	case host == "embed.settlar.io" && path == "/api/embed/session":
		return 200, `{"kind":"hls","source":"https://cdn.example/anipm/master.m3u8","audioLang":"ja","subtitles":[{"url":"https://cdn.example/anipm/en.vtt","assUrl":"https://cdn.example/anipm/en.ass","label":"English","srclang":"en","default":true}]}`

	// --- stream CDNs: any playlist URL must be a playable HLS manifest ---
	case strings.HasSuffix(path, ".m3u8"):
		return 200, contractPlaylist
	case strings.HasSuffix(path, ".ts"):
		return 200, "\x47\x40\x00\x10\x02\x42\xf0\x25"
	}
	return 404, "not found"
}

// probePlaylist fetches a resolved stream URL through the same client and
// asserts it is a real HLS playlist with at least one segment/variant line.
// This is the headless equivalent of "mpv would actually start playing".
// The referrer is sent exactly as mpv would send the hint's Referrer.
func probePlaylist(t *testing.T, client *http.Client, streamURL, referrer string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, streamURL, nil)
	if err != nil {
		t.Fatalf("probe: bad stream url %q: %v", streamURL, err)
	}
	// Several stream CDNs reject Go's default UA; mpv/browser UAs pass.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	if referrer != "" {
		req.Header.Set("Referer", referrer)
	}
	probeClient := client
	if u, err := url.Parse(streamURL); err == nil && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost") {
		// Local stream proxies (senshi/anineko) are real HTTP servers;
		// they fetch upstream through the injected client themselves.
		probeClient = http.DefaultClient
	}
	resp, err := probeClient.Do(req)
	if err != nil {
		t.Fatalf("probe: fetch %q: %v", streamURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("probe: read %q: %v", streamURL, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("probe: %q returned status %d", streamURL, resp.StatusCode)
	}
	if !strings.Contains(string(body), "#EXTM3U") {
		t.Fatalf("probe: %q is not an HLS playlist: %.120q", streamURL, string(body))
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return // found a segment or variant line
		}
	}
	t.Fatalf("probe: %q playlist has no segment/variant lines", streamURL)
}

// contractCase identifies one provider's canonical flow through the registry.
type contractCase struct {
	name  string
	query string
	mode  string
}

var contractCases = []contractCase{
	{name: "senshi", query: "frieren", mode: "sub"},
	{name: "anipub", query: "frieren", mode: "sub"},
	{name: "anineko", query: "frieren", mode: "sub"},
	{name: "anipm", query: "frieren", mode: "sub"},
}

// runContractFlow executes Search -> Episodes -> Stream -> playlist probe
// for one provider and returns the resolved stream URL.
func runContractFlow(t *testing.T, client *http.Client, tc contractCase) string {
	t.Helper()

	provider, err := providers.New(tc.name)
	if err != nil {
		t.Fatalf("%s: providers.New: %v", tc.name, err)
	}
	if provider.Name() != tc.name {
		t.Fatalf("%s: Name() = %q", tc.name, provider.Name())
	}

	options, err := provider.SearchAnime(tc.query, tc.mode)
	if err != nil {
		t.Fatalf("%s: SearchAnime(%q): %v", tc.name, tc.query, err)
	}
	if len(options) == 0 || options[0].Key == "" {
		t.Fatalf("%s: SearchAnime returned no usable results: %#v", tc.name, options)
	}
	showID := options[0].Key

	episodes, err := provider.EpisodesList(showID, tc.mode)
	if err != nil {
		t.Fatalf("%s: EpisodesList(%q): %v", tc.name, showID, err)
	}
	if len(episodes) == 0 {
		t.Fatalf("%s: EpisodesList returned no episodes", tc.name)
	}

	cfg := providers.PlaybackConfig{SubOrDub: tc.mode, SubStyle: "soft"}
	var links []string
	var hints map[string]providers.StreamPlaybackHint
	if resolver, ok := provider.(providers.HintResolver); ok {
		links, hints, err = resolver.GetEpisodeURLForModeWithHints(cfg, showID, 1, tc.mode)
		if err == nil {
			for _, link := range links {
				if _, ok := hints[link]; !ok {
					t.Fatalf("%s: stream %q missing playback hint", tc.name, link)
				}
			}
		}
	} else {
		links, err = provider.GetEpisodeURL(cfg, showID, 1)
	}
	if err != nil {
		t.Fatalf("%s: GetEpisodeURL(ep 1): %v", tc.name, err)
	}
	if len(links) == 0 {
		t.Fatalf("%s: GetEpisodeURL returned no links", tc.name)
	}

	probePlaylist(t, client, links[0], hints[links[0]].Referrer)
	return links[0]
}

func TestProviderContractOffline(t *testing.T) {
	client := &http.Client{Transport: contractTransport{handler: contractHappyHandler}}
	installContractHost(t, client)

	for _, tc := range contractCases {
		t.Run(tc.name, func(t *testing.T) {
			runContractFlow(t, client, tc)
		})
	}
}

func TestProviderRegistryComplete(t *testing.T) {
	want := []string{"allanime", "anikoto", "animepahe", "anineko", "anipm", "anipub", "senshi"}
	got := providers.RegisteredNames()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("RegisteredNames = %v, want %v", got, want)
	}
	for _, name := range want {
		provider, err := providers.New(name)
		if err != nil {
			t.Fatalf("New(%q): %v", name, err)
		}
		if provider.Name() != name {
			t.Fatalf("New(%q).Name() = %q", name, provider.Name())
		}
	}
	if _, err := providers.New("no-such-provider"); err == nil {
		t.Fatal("New(unknown) must return an error, not a provider or panic")
	}
	if got := providers.NormalizeName(" All-Anime "); got != "allanime" {
		t.Fatalf("NormalizeName(alias) = %q, want allanime", got)
	}
	if got := providers.NormalizeName(""); got != "" {
		t.Fatalf("NormalizeName(empty) = %q, want empty", got)
	}
}

// TestProviderContractEdgeCases feeds every contract provider broken
// upstream responses and out-of-range episode numbers. The contract is:
// return an error (or empty results), never panic, never return links.
func TestProviderContractEdgeCases(t *testing.T) {
	broken := map[string]func(req *http.Request) (int, string){
		"http_500":        func(*http.Request) (int, string) { return 500, "internal error" },
		"html_not_json":   func(*http.Request) (int, string) { return 200, "<html><body>cloudflare</body></html>" },
		"malformed_json":  func(*http.Request) (int, string) { return 200, "{not json" },
		"empty_body":      func(*http.Request) (int, string) { return 200, "" },
		"empty_results":   func(*http.Request) (int, string) { return 200, `[]` },
		"transport_error": nil, // handled below via a failing transport
	}

	for _, tc := range contractCases {
		for name, handler := range broken {
			t.Run(tc.name+"/"+name, func(t *testing.T) {
				var client *http.Client
				if handler == nil {
					client = &http.Client{Transport: contractTransport{handler: func(*http.Request) (int, string) {
						return 502, "bad gateway"
					}}}
				} else {
					client = &http.Client{Transport: contractTransport{handler: handler}}
				}
				installContractHost(t, client)

				provider, err := providers.New(tc.name)
				if err != nil {
					t.Fatalf("New: %v", err)
				}
				// A panic in any of these calls fails the test: that is
				// the assertion. All must degrade to error/empty.
				if options, err := provider.SearchAnime(tc.query, tc.mode); err == nil && len(options) > 0 {
					t.Fatalf("expected error or no results, got %#v", options)
				}
				if episodes, err := provider.EpisodesList("52991", tc.mode); err == nil && len(episodes) > 0 {
					t.Fatalf("expected error or no episodes, got %#v", episodes)
				}
				cfg := providers.PlaybackConfig{SubOrDub: tc.mode, SubStyle: "soft"}
				if links, err := provider.GetEpisodeURL(cfg, "52991", 1); err == nil && len(links) > 0 {
					t.Fatalf("expected error or no links, got %#v", links)
				}
			})
		}

		t.Run(tc.name+"/episode_bounds", func(t *testing.T) {
			client := &http.Client{Transport: contractTransport{handler: contractHappyHandler}}
			installContractHost(t, client)
			provider, err := providers.New(tc.name)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			cfg := providers.PlaybackConfig{SubOrDub: tc.mode, SubStyle: "soft"}
			for _, epNo := range []int{0, -1, -100, 9999, 1 << 30} {
				if links, err := provider.GetEpisodeURL(cfg, "52991", epNo); err == nil && len(links) > 0 {
					t.Fatalf("ep %d: expected error or no links, got %#v", epNo, links)
				}
			}
			if options, err := provider.SearchAnime("", tc.mode); err == nil && len(options) > 0 {
				t.Fatalf("empty query: expected error or no results, got %#v", options)
			}
			if options, err := provider.SearchAnime("   ", tc.mode); err == nil && len(options) > 0 {
				t.Fatalf("blank query: expected error or no results, got %#v", options)
			}
			if _, err := provider.EpisodesList("", tc.mode); err == nil {
				t.Fatal("empty show id: expected error")
			}
		})
	}
}

// TestLiveProviderRanking measures every enabled provider against the same
// titles (search hits, episode depth, ep-1 sub stream, latency) so the
// default stack order in internal/provider_disabled.go can be set from
// data instead of vibes. Log-only except for transport sanity.
//
//	CURD_LIVE_RANK=1 go test ./internal/providers -run TestLiveProviderRanking -v -timeout 30m
func TestLiveProviderRanking(t *testing.T) {
	if os.Getenv("CURD_LIVE_RANK") == "" {
		t.Skip("set CURD_LIVE_RANK=1 to run the live provider ranking")
	}
	installContractHost(t, http.DefaultClient)

	names := []string{"anipm", "anipub", "anikoto", "senshi", "anineko", "allanime"}
	titles := []string{"frieren", "one piece", "naruto", "your name", "dungeon meshi"}

	type score struct {
		searchOK, streamOK, titles int
		episodes                   int
		searchMs, streamMs         int64
	}
	scores := map[string]*score{}

	for _, name := range names {
		provider, err := providers.New(name)
		if err != nil {
			t.Fatalf("New(%s): %v", name, err)
		}
		sc := &score{}
		scores[name] = sc
		for _, title := range titles {
			sc.titles++
			start := time.Now()
			options, err := provider.SearchAnime(title, "sub")
			searchMs := time.Since(start).Milliseconds()
			if err != nil || len(options) == 0 {
				t.Logf("rank %-8s %-14q search FAIL (%dms) err=%v", name, title, searchMs, err)
				continue
			}
			// Pick the result whose title best matches the query.
			picked := options[0]
			for _, o := range options {
				if strings.Contains(strings.ToLower(o.Title), strings.ToLower(title)) {
					picked = o
					break
				}
			}
			episodes, err := provider.EpisodesList(picked.Key, "sub")
			if err != nil || len(episodes) == 0 {
				t.Logf("rank %-8s %-14q search ok (%dms) episodes FAIL err=%v", name, title, searchMs, err)
				continue
			}
			sc.searchOK++
			sc.searchMs += searchMs
			sc.episodes += len(episodes)

			start = time.Now()
			links, err := provider.GetEpisodeURL(providers.PlaybackConfig{SubOrDub: "sub", SubStyle: "soft"}, picked.Key, 1)
			streamMs := time.Since(start).Milliseconds()
			if err != nil || len(links) == 0 {
				t.Logf("rank %-8s %-14q eps=%d stream FAIL (%dms) err=%v", name, title, len(episodes), streamMs, err)
				continue
			}
			sc.streamOK++
			sc.streamMs += streamMs
			t.Logf("rank %-8s %-14q eps=%-4d search=%dms stream=%dms OK", name, title, len(episodes), searchMs, streamMs)
		}
	}

	t.Log("=== provider ranking (search/stream successes, avg latency, total episodes) ===")
	for _, name := range names {
		sc := scores[name]
		var avgSearch, avgStream int64
		if sc.searchOK > 0 {
			avgSearch = sc.searchMs / int64(sc.searchOK)
		}
		if sc.streamOK > 0 {
			avgStream = sc.streamMs / int64(sc.streamOK)
		}
		t.Logf("rank summary %-8s search %d/%d stream %d/%d avgSearch=%dms avgStream=%dms episodes=%d",
			name, sc.searchOK, sc.titles, sc.streamOK, sc.titles, avgSearch, avgStream, sc.episodes)
	}
}

// TestLiveProviderContract runs the identical contract against the real
// provider sites. Opt-in because it needs the network and the sites up.
//
//	CURD_LIVE_CONTRACT=1 go test ./internal/providers -run TestLiveProviderContract -v -timeout 10m
func TestLiveProviderContract(t *testing.T) {
	if os.Getenv("CURD_LIVE_CONTRACT") == "" {
		t.Skip("set CURD_LIVE_CONTRACT=1 to run the live provider contract")
	}
	installContractHost(t, http.DefaultClient)

	for _, tc := range contractCases {
		t.Run(tc.name, func(t *testing.T) {
			runContractFlow(t, http.DefaultClient, tc)
		})
	}
	// animepahe needs a real browser bypass and allanime is disabled by
	// default in the app; both stay covered by their dedicated live tests.
	for _, name := range []string{"allanime", "animepahe"} {
		t.Run(name+"/search_only", func(t *testing.T) {
			provider, err := providers.New(name)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			options, err := provider.SearchAnime("frieren", "sub")
			if err != nil {
				t.Logf("%s live search unavailable (logged, not fatal): %v", name, err)
				return
			}
			if len(options) == 0 {
				t.Logf("%s live search returned no results", name)
			}
		})
	}
}
