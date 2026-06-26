package providers_test

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/wraient/curd/internal/curdhost"
	_ "github.com/wraient/curd/internal/loadproviders"
	"github.com/wraient/curd/internal/providers"
	"github.com/wraient/curd/internal/providers/anineko"
	"github.com/wraient/curd/internal/providers/anipub"
	"github.com/wraient/curd/internal/providers/senshi"
)

// Run with:
//
//	CURD_LIVE_EDGE=1 go test ./internal/providers -run TestLiveProviderEdgeCases -v -timeout 45m
func initLiveEdge(tb testing.TB) {
	tb.Helper()
	if os.Getenv("CURD_LIVE_EDGE") == "" {
		tb.Skip("set CURD_LIVE_EDGE=1 to run live provider edge-case tests")
	}
	if curdhost.HTTPClient == nil {
		curdhost.HTTPClient = func() *http.Client { return http.DefaultClient }
	}
}

type edgeAnimeCase struct {
	name        string
	query       string
	wantInTitle string
}

var edgeAnimeCases = []edgeAnimeCase{
	// punctuation & symbols (how users actually type)
	{name: "colon_apostrophe", query: "frieren beyond journey's end", wantInTitle: "frieren"},
	{name: "colon_short", query: "re:zero", wantInTitle: "re:zero"},
	{name: "colon_spaced", query: "re zero", wantInTitle: "re:zero"},
	{name: "semicolon", query: "steins;gate", wantInTitle: "stein"},
	{name: "slash", query: "fate/stay night", wantInTitle: "fate"},
	{name: "bang", query: "k-on!", wantInTitle: "k-on"},
	{name: "double_bang", query: "love live superstar", wantInTitle: "love live"},

	// apostrophe variants
	{name: "apostrophe_full", query: "JoJo's Bizarre Adventure", wantInTitle: "jojo"},
	{name: "apostrophe_stripped", query: "jojos bizarre adventure", wantInTitle: "jojo"},

	// romaji vs english
	{name: "romaji_snk", query: "shingeki no kyojin", wantInTitle: "attack on titan"},
	{name: "english_aot", query: "attack on titan", wantInTitle: "attack on titan"},
	{name: "romaji_mha", query: "boku no hero academia", wantInTitle: "hero academia"},
	{name: "english_mha", query: "my hero academia", wantInTitle: "hero academia"},
	{name: "romaji_frieren", query: "sousou no frieren", wantInTitle: "frieren"},

	// short / numeric queries
	{name: "numeric_86", query: "86", wantInTitle: "86"},
	{name: "two_digit", query: "91 days", wantInTitle: "91 days"},
	{name: "three_letters", query: "mha", wantInTitle: "hero"},

	// x-in-title
	{name: "x_hxh", query: "hunter x hunter", wantInTitle: "hunter"},
	{name: "x_spy", query: "spy x family", wantInTitle: "spy"},

	// long titles (truncated user search)
	{name: "long_magirevo", query: "magical revolution reincarnated princess", wantInTitle: "magical revolution"},
	{name: "long_slime", query: "that time i got reincarnated as a slime", wantInTitle: "slime"},
	{name: "long_apothecary", query: "apothecary diaries", wantInTitle: "apothecary"},

	// ambiguous / crowded results
	{name: "ambiguous_monogatari", query: "monogatari", wantInTitle: "monogatari"},
	{name: "ambiguous_fate", query: "fate", wantInTitle: "fate"},
	{name: "ambiguous_gundam", query: "gundam", wantInTitle: "gundam"},

	// season / cour noise
	{name: "season_noise", query: "frieren season 2", wantInTitle: "frieren"},
	{name: "cour_alias", query: "mushoku tensei jobless reincarnation", wantInTitle: "mushoku"},

	// spinoffs & similar titles
	{name: "spinoff_mini", query: "frieren marumaru mahou", wantInTitle: "frieren"},
	{name: "long_runner", query: "one piece", wantInTitle: "one piece"},

	// abbreviations & nicknames
	{name: "abbr_sao", query: "sao", wantInTitle: "sword art"},
	{name: "abbr_danmachi", query: "danmachi", wantInTitle: "danmachi"},
	{name: "abbr_csm", query: "chainsaw man", wantInTitle: "chainsaw"},

	// movies & one-shots
	{name: "movie_your_name", query: "your name", wantInTitle: "your name"},
	{name: "movie_suzume", query: "suzume", wantInTitle: "suzume"},

	// recent popular
	{name: "oshi_no_ko", query: "oshi no ko", wantInTitle: "oshi"},
	{name: "bocchi", query: "bocchi the rock", wantInTitle: "bocchi"},
	{name: "dungeon_meshi", query: "dungeon meshi", wantInTitle: "dungeon"},
	{name: "delicious_in_dungeon", query: "delicious in dungeon", wantInTitle: "dungeon"},

	// edge punctuation stripped
	{name: "bleach_tybw", query: "bleach thousand year blood war", wantInTitle: "bleach"},
	{name: "evangelion", query: "neon genesis evangelion", wantInTitle: "evangelion"},

	// mild typo / spacing
	{name: "demon_slayer", query: "demon slayer", wantInTitle: "demon"},
	{name: "kimetsu", query: "kimetsu no yaiba", wantInTitle: "demon"},

	// unicode-ish (user without diacritics)
	{name: "witch_hat", query: "witch hat atelier", wantInTitle: "witch"},

	// single word common
	{name: "single_naruto", query: "naruto", wantInTitle: "naruto"},
	{name: "single_bleach", query: "bleach", wantInTitle: "bleach"},
}

type edgeProvider struct {
	name     string
	search   func(string, string) ([]providers.SelectionOption, error)
	episodes func(string, string) ([]string, error)
	streams  func(string, providers.PlaybackConfig, int) ([]string, map[string]providers.StreamPlaybackHint, error)
}

func edgeProviders() []edgeProvider {
	return []edgeProvider{
		{
			name: "senshi",
			search: func(q, mode string) ([]providers.SelectionOption, error) {
				return (&senshi.Provider{}).SearchAnime(q, mode)
			},
			episodes: func(id, mode string) ([]string, error) {
				return (&senshi.Provider{}).EpisodesList(id, mode)
			},
			streams: func(id string, cfg providers.PlaybackConfig, ep int) ([]string, map[string]providers.StreamPlaybackHint, error) {
				return (&senshi.Provider{}).GetEpisodeURLForModeWithHints(cfg, id, ep, cfg.SubOrDub)
			},
		},
		{
			name: "anipub",
			search: func(q, mode string) ([]providers.SelectionOption, error) {
				return (&anipub.Provider{}).SearchAnime(q, mode)
			},
			episodes: func(id, mode string) ([]string, error) {
				return (&anipub.Provider{}).EpisodesList(id, mode)
			},
			streams: func(id string, cfg providers.PlaybackConfig, ep int) ([]string, map[string]providers.StreamPlaybackHint, error) {
				return (&anipub.Provider{}).GetEpisodeURLForModeWithHints(cfg, id, ep, cfg.SubOrDub)
			},
		},
		{
			name: "anineko",
			search: func(q, mode string) ([]providers.SelectionOption, error) {
				return (&anineko.Provider{}).SearchAnime(q, mode)
			},
			episodes: func(id, mode string) ([]string, error) {
				return (&anineko.Provider{}).EpisodesList(id, mode)
			},
			streams: func(id string, cfg providers.PlaybackConfig, ep int) ([]string, map[string]providers.StreamPlaybackHint, error) {
				return (&anineko.Provider{}).GetEpisodeURLForModeWithHints(cfg, id, ep, cfg.SubOrDub)
			},
		},
	}
}

type edgeResult struct {
	provider    string
	caseName    string
	query       string
	searchOK    bool
	matchOK     bool
	episodesOK  bool
	streamOK    bool
	searchErr   string
	episodesErr string
	streamErr   string
	pickedTitle string
	pickedKey   string
	epCount     int
	searchMs    int64
	flowMs      int64
}

func pickEdgeResult(options []providers.SelectionOption, want string) (providers.SelectionOption, bool) {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" && len(options) > 0 {
		return options[0], true
	}
	for _, option := range options {
		for _, candidate := range []string{option.Title, option.Label} {
			if strings.Contains(strings.ToLower(candidate), want) {
				return option, true
			}
		}
	}
	if len(options) > 0 {
		return options[0], false
	}
	return providers.SelectionOption{}, false
}

func runEdgeCase(provider edgeProvider, tc edgeAnimeCase) edgeResult {
	res := edgeResult{
		provider: provider.name,
		caseName: tc.name,
		query:    tc.query,
	}

	searchStart := time.Now()
	options, err := provider.search(tc.query, "sub")
	res.searchMs = time.Since(searchStart).Milliseconds()
	if err != nil {
		res.searchErr = err.Error()
		return res
	}
	if len(options) == 0 {
		res.searchErr = "no results"
		return res
	}
	res.searchOK = true

	picked, matched := pickEdgeResult(options, tc.wantInTitle)
	res.pickedTitle = picked.Title
	if res.pickedTitle == "" {
		res.pickedTitle = picked.Label
	}
	res.pickedKey = picked.Key
	res.matchOK = matched

	if provider.episodes == nil || provider.streams == nil || picked.Key == "" {
		return res
	}

	flowStart := time.Now()
	eps, err := provider.episodes(picked.Key, "sub")
	if err != nil {
		res.episodesErr = err.Error()
		res.flowMs = time.Since(flowStart).Milliseconds()
		return res
	}
	if len(eps) == 0 {
		res.episodesErr = "no episodes"
		res.flowMs = time.Since(flowStart).Milliseconds()
		return res
	}
	res.episodesOK = true
	res.epCount = len(eps)

	links, _, err := provider.streams(picked.Key, providers.PlaybackConfig{SubOrDub: "sub"}, 1)
	res.flowMs = time.Since(flowStart).Milliseconds()
	if err != nil {
		res.streamErr = err.Error()
		return res
	}
	if len(links) == 0 {
		res.streamErr = "no stream url"
		return res
	}
	res.streamOK = true
	return res
}

func TestLiveProviderEdgeCases(t *testing.T) {
	initLiveEdge(t)

	providersList := edgeProviders()
	results := make([]edgeResult, 0, len(edgeAnimeCases)*len(providersList))

	for _, provider := range providersList {
		for _, tc := range edgeAnimeCases {
			res := runEdgeCase(provider, tc)
			results = append(results, res)
			t.Logf("%s", edgeStatusLine(res))
			time.Sleep(150 * time.Millisecond)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].provider != results[j].provider {
			return results[i].provider < results[j].provider
		}
		return results[i].caseName < results[j].caseName
	})

	t.Log(edgeSummary(results))

	type counts struct{ search, match, episodes, stream, total int }
	byProvider := map[string]counts{}
	for _, res := range results {
		c := byProvider[res.provider]
		c.total++
		if res.searchOK {
			c.search++
		}
		if res.matchOK {
			c.match++
		}
		if res.episodesOK {
			c.episodes++
		}
		if res.streamOK {
			c.stream++
		}
		byProvider[res.provider] = c
	}

	for name, c := range byProvider {
		t.Logf("%s: search %d/%d match %d/%d episodes %d/%d stream %d/%d",
			name, c.search, c.total, c.match, c.total, c.episodes, c.total, c.stream, c.total)
	}

	failures := make([]string, 0)
	for _, res := range results {
		if !res.searchOK {
			failures = append(failures, fmt.Sprintf("%s/%s search failed: %s", res.provider, res.caseName, res.searchErr))
			continue
		}
		if !res.matchOK {
			failures = append(failures, fmt.Sprintf("%s/%s weak match: picked %q for query %q", res.provider, res.caseName, res.pickedTitle, res.query))
		}
		if res.searchOK && !res.streamOK {
			failures = append(failures, fmt.Sprintf("%s/%s stream failed: episodes_err=%s stream_err=%s", res.provider, res.caseName, res.episodesErr, res.streamErr))
		}
	}

	critical := []string{"english_aot", "single_naruto", "demon_slayer", "dungeon_meshi", "romaji_frieren"}
	for _, provider := range []string{"senshi", "anineko"} {
		for _, caseName := range critical {
			for _, res := range results {
				if res.provider == provider && res.caseName == caseName {
					if !res.streamOK {
						t.Errorf("critical case %s/%s must stream: search=%v episodes=%v stream_err=%s", provider, caseName, res.searchOK, res.episodesOK, res.streamErr)
					}
					break
				}
			}
		}
	}

	if len(failures) > 0 {
		t.Logf("%d edge-case issues logged (see above)", len(failures))
	}
}

func edgeStatusLine(res edgeResult) string {
	match := "ok"
	if res.searchOK && !res.matchOK {
		match = "weak"
	}
	if !res.searchOK {
		match = "fail"
	}
	stream := "-"
	if res.streamOK {
		stream = "ok"
	} else if res.searchOK {
		stream = "fail"
	}
	return fmt.Sprintf("[%s] %-22s %-28s search=%s match=%s eps=%v stream=%s (%dms+%dms) picked=%q",
		res.provider, res.caseName, res.query,
		boolMark(res.searchOK), match, res.episodesOK, stream,
		res.searchMs, res.flowMs, truncate(res.pickedTitle, 48))
}

func edgeSummary(results []edgeResult) string {
	var b strings.Builder
	b.WriteString("\n=== provider edge-case summary ===\n")
	current := ""
	for _, res := range results {
		if res.provider != current {
			current = res.provider
			b.WriteString("\n-- " + current + " --\n")
		}
		b.WriteString(edgeStatusLine(res))
		b.WriteByte('\n')
	}
	return b.String()
}

func boolMark(ok bool) string {
	if ok {
		return "ok"
	}
	return "FAIL"
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
