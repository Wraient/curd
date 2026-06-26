package providers_test

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/wraient/curd/internal/curdhost"
	_ "github.com/wraient/curd/internal/loadproviders"
	"github.com/wraient/curd/internal/providers"
)

// Run with:
//
//	CURD_LIVE_MOVIES=1 go test ./internal/providers -run TestLiveProviderMovies -v -timeout 30m
func initLiveMovies(tb testing.TB) {
	tb.Helper()
	if os.Getenv("CURD_LIVE_MOVIES") == "" {
		tb.Skip("set CURD_LIVE_MOVIES=1 to run live movie provider tests")
	}
	if curdhost.HTTPClient == nil {
		curdhost.HTTPClient = func() *http.Client { return http.DefaultClient }
	}
}

type movieCase struct {
	name        string
	query       string
	wantInTitle string
}

var movieCases = []movieCase{
	{name: "your_name", query: "your name", wantInTitle: "your name"},
	{name: "your_name_romaji", query: "kimi no na wa", wantInTitle: "your name"},
	{name: "suzume", query: "suzume", wantInTitle: "suzume"},
	{name: "spirited_away", query: "spirited away", wantInTitle: "spirited"},
	{name: "silent_voice", query: "a silent voice", wantInTitle: "silent"},
	{name: "weathering", query: "weathering with you", wantInTitle: "weather"},
	{name: "mugen_train", query: "demon slayer mugen train", wantInTitle: "mugen"},
	{name: "jjk_0", query: "jujutsu kaisen 0", wantInTitle: "jujutsu kaisen 0"},
	{name: "film_red", query: "one piece film red", wantInTitle: "red"},
	{name: "howl", query: "howl's moving castle", wantInTitle: "howl"},
	{name: "grave_fireflies", query: "grave of the fireflies", wantInTitle: "grave"},
	{name: "akira", query: "akira", wantInTitle: "akira"},
	{name: "perfect_blue", query: "perfect blue", wantInTitle: "perfect"},
	{name: "promare", query: "promare", wantInTitle: "promare"},
	{name: "look_back", query: "look back", wantInTitle: "look back"},
	{name: "reze_arc", query: "chainsaw man reze arc", wantInTitle: "chainsaw"},
	{name: "paprika", query: "paprika", wantInTitle: "paprika"},
	{name: "girl_who_leapt", query: "girl who leapt through time", wantInTitle: "leapt"},
}

type movieResult struct {
	provider, caseName, query string
	searchOK, matchOK, epListOK, streamOK bool
	epCount                                 int
	pickedTitle, pickedKey                  string
	searchErr, episodesErr, streamErr       string
}

func runMovieCase(p edgeProvider, tc movieCase) movieResult {
	res := movieResult{provider: p.name, caseName: tc.name, query: tc.query}
	opts, err := p.search(tc.query, "sub")
	if err != nil {
		res.searchErr = err.Error()
		return res
	}
	if len(opts) == 0 {
		res.searchErr = "no results"
		return res
	}
	res.searchOK = true
	picked, matched := pickEdgeResult(opts, tc.wantInTitle)
	res.matchOK = matched
	res.pickedTitle = picked.Title
	if res.pickedTitle == "" {
		res.pickedTitle = picked.Label
	}
	res.pickedKey = picked.Key
	if picked.Key == "" {
		return res
	}
	eps, err := p.episodes(picked.Key, "sub")
	if err != nil {
		res.episodesErr = err.Error()
	} else if len(eps) == 0 {
		res.episodesErr = "no episodes"
	} else {
		res.epListOK = true
		res.epCount = len(eps)
	}
	links, _, err := p.streams(picked.Key, providers.PlaybackConfig{SubOrDub: "sub"}, 1)
	if err != nil {
		res.streamErr = err.Error()
	} else if len(links) == 0 {
		res.streamErr = "no stream url"
	} else {
		res.streamOK = true
	}
	return res
}

func TestLiveProviderMovies(t *testing.T) {
	initLiveMovies(t)

	type counts struct{ search, match, epList, stream, total int }
	byProvider := map[string]counts{}

	for _, p := range edgeProviders() {
		for _, tc := range movieCases {
			res := runMovieCase(p, tc)
			c := byProvider[res.provider]
			c.total++
			if res.searchOK {
				c.search++
			}
			if res.matchOK {
				c.match++
			}
			if res.epListOK {
				c.epList++
			}
			if res.streamOK {
				c.stream++
			}
			byProvider[res.provider] = c

			epNote := "no-list"
			if res.epListOK {
				epNote = fmt.Sprintf("%deps", res.epCount)
			}
			t.Logf("[%s] %-20s %-30s search=%s match=%s eps=%s stream=%s picked=%q",
				res.provider, res.caseName, res.query,
				boolMark(res.searchOK), boolMark(res.matchOK), epNote, boolMark(res.streamOK),
				truncate(res.pickedTitle, 40))
			time.Sleep(200 * time.Millisecond)
		}
	}

	for name, c := range byProvider {
		t.Logf("%s movies: search %d/%d match %d/%d episode_list %d/%d stream %d/%d",
			name, c.search, c.total, c.match, c.total, c.epList, c.total, c.stream, c.total)
	}
}
