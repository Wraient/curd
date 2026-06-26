package providers_test

import (
	"net/http"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/wraient/curd/internal/curdhost"
	_ "github.com/wraient/curd/internal/loadproviders"
	"github.com/wraient/curd/internal/providers"
	"github.com/wraient/curd/internal/providers/anipub"
	"github.com/wraient/curd/internal/providers/anineko"
	"github.com/wraient/curd/internal/providers/senshi"
)

const liveBenchQuery = "frieren"

var liveBenchCases = []struct {
	name      string
	provider  string
	showID    string
	epNo      int
	mode      string
	searchFn  func(string, string) ([]providers.SelectionOption, error)
	episodes  func(string, string) ([]string, error)
	streams   func(string, providers.PlaybackConfig, int) ([]string, map[string]providers.StreamPlaybackHint, error)
}{
	{
		name:     "senshi",
		provider: "senshi",
		showID:   "52991",
		epNo:     1,
		mode:     "sub",
		searchFn: func(query, mode string) ([]providers.SelectionOption, error) {
			return senshiSearch(query, mode)
		},
		episodes: senshiEpisodes,
		streams:  senshiStreams,
	},
	{
		name:     "anipub",
		provider: "anipub",
		showID:   "2454",
		epNo:     1,
		mode:     "sub",
		searchFn: anipubSearch,
		episodes: anipubEpisodes,
		streams:  anipubStreams,
	},
	{
		name:     "anineko",
		provider: "anineko",
		showID:   "",
		epNo:     1,
		mode:     "sub",
		searchFn: aninekoSearch,
		episodes: nil,
		streams:  nil,
	},
}

func senshiSearch(query, mode string) ([]providers.SelectionOption, error) {
	p := &senshi.Provider{}
	return p.SearchAnime(query, mode)
}

func senshiEpisodes(showID, mode string) ([]string, error) {
	p := &senshi.Provider{}
	return p.EpisodesList(showID, mode)
}

func senshiStreams(showID string, config providers.PlaybackConfig, epNo int) ([]string, map[string]providers.StreamPlaybackHint, error) {
	p := &senshi.Provider{}
	return p.GetEpisodeURLForModeWithHints(config, showID, epNo, config.SubOrDub)
}

func anipubSearch(query, mode string) ([]providers.SelectionOption, error) {
	p := &anipub.Provider{}
	return p.SearchAnime(query, mode)
}

func anipubEpisodes(showID, mode string) ([]string, error) {
	p := &anipub.Provider{}
	return p.EpisodesList(showID, mode)
}

func anipubStreams(showID string, config providers.PlaybackConfig, epNo int) ([]string, map[string]providers.StreamPlaybackHint, error) {
	p := &anipub.Provider{}
	return p.GetEpisodeURLForModeWithHints(config, showID, epNo, config.SubOrDub)
}

func aninekoSearch(query, mode string) ([]providers.SelectionOption, error) {
	p := &anineko.Provider{}
	return p.SearchAnime(query, mode)
}

func initLiveBench(tb testing.TB) {
	tb.Helper()
	if os.Getenv("CURD_LIVE_BENCH") == "" {
		tb.Skip("set CURD_LIVE_BENCH=1 to run live benchmarks")
	}
	if curdhost.HTTPClient == nil {
		curdhost.HTTPClient = func() *http.Client { return http.DefaultClient }
	}
}

func BenchmarkProviderSearch(b *testing.B) {
	initLiveBench(b)
	for _, tc := range liveBenchCases {
		if tc.searchFn == nil {
			continue
		}
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := tc.searchFn(liveBenchQuery, tc.mode); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkProviderEpisodeStream(b *testing.B) {
	initLiveBench(b)
	for _, tc := range liveBenchCases {
		if tc.episodes == nil || tc.streams == nil {
			continue
		}
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := tc.episodes(tc.showID, tc.mode); err != nil {
					b.Fatal(err)
				}
				if _, _, err := tc.streams(tc.showID, providers.PlaybackConfig{SubOrDub: tc.mode}, tc.epNo); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestLiveProviderFlowLatency(t *testing.T) {
	initLiveBench(t)

	type result struct {
		name   string
		search time.Duration
		flow   time.Duration
	}

	results := make([]result, 0, len(liveBenchCases))
	for _, tc := range liveBenchCases {
		if tc.searchFn == nil || tc.episodes == nil || tc.streams == nil {
			continue
		}

		searchStart := time.Now()
		options, err := tc.searchFn(liveBenchQuery, tc.mode)
		if err != nil {
			t.Fatalf("%s search: %v", tc.name, err)
		}
		searchLatency := time.Since(searchStart)

		showID := tc.showID
		if showID == "" {
			for _, option := range options {
				if option.Key != "" {
					showID = option.Key
					break
				}
			}
		}
		if showID == "" {
			t.Fatalf("%s: no show id for flow benchmark", tc.name)
		}

		flowStart := time.Now()
		if _, err := tc.episodes(showID, tc.mode); err != nil {
			t.Fatalf("%s episodes: %v", tc.name, err)
		}
		links, _, err := tc.streams(showID, providers.PlaybackConfig{SubOrDub: tc.mode}, tc.epNo)
		if err != nil {
			t.Fatalf("%s streams: %v", tc.name, err)
		}
		if len(links) == 0 {
			t.Fatalf("%s: no stream links", tc.name)
		}
		flowLatency := time.Since(flowStart)

		results = append(results, result{name: tc.name, search: searchLatency, flow: searchLatency + flowLatency})
		t.Logf("%s search=%s full_flow=%s", tc.name, searchLatency, searchLatency+flowLatency)
	}

	if len(results) < 2 {
		t.Skip("not enough providers with full flow support")
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].flow < results[j].flow
	})
	fastest := results[0].name
	t.Logf("fastest full flow: %s (%s)", fastest, results[0].flow)

	for _, item := range results[1:] {
		if item.flow > results[0].flow*3 {
			t.Logf("warning: %s full flow is much slower than %s (%s vs %s)", item.name, fastest, item.flow, results[0].flow)
		}
	}
}
