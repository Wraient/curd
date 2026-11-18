package internal

// Hard-anime benchmark suite: 27 deliberately difficult titles (sequels,
// remakes, same-name shows, punctuation, typos, long runners, movies,
// numeric/short queries) run against the live provider stack.
//
// It measures, per title: stacked search hits, auto-match correctness
// (picked title must contain a known-good fragment — a wrong show is a
// FAIL), sub stream resolution for episode 1, and latency. It separates
// provider gaps (no results / no streams anywhere) from matching gaps
// (results exist but the wrong one is picked or nothing auto-matches).
//
// Run with:
//
//	CURD_LIVE_HARD=1 go test ./internal -run TestHardAnimeBenchmark -v -count=1 -timeout 30m
//
// The suite is opt-in because it hits live provider sites. Be polite:
// cases are spaced with a short delay to avoid rate-limiting providers.

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wraient/curd/internal/curdhost"
)

type hardAnimeCase struct {
	name       string
	query      string
	frags      []string // any-of: picked title must contain one (case-insensitive)
	episodes   int      // tracker episode count; 0 = unknown
	format     string   // tracker Media.Format; "" = unknown
	malID      int      // tracker MAL ID when certain; 0 = unknown
	noResultOK bool     // typos: providers may return nothing usable at all
}

var hardAnimeCases = []hardAnimeCase{
	{name: "frieren_s2", query: "frieren season 2", frags: []string{"frieren"}, episodes: 0, format: "TV"},
	{name: "jojo_2012", query: "JoJo's Bizarre Adventure (2012)", frags: []string{"jojo", "jo jo"}, episodes: 26, format: "TV", malID: 14719},
	{name: "fruits_basket", query: "fruits basket", frags: []string{"fruits basket"}, episodes: 25, format: "TV", malID: 38671},
	{name: "hxh", query: "hunter x hunter", frags: []string{"hunter"}, episodes: 148, format: "TV", malID: 11061},
	{name: "fmab", query: "fullmetal alchemist brotherhood", frags: []string{"fullmetal"}, episodes: 64, format: "TV", malID: 5114},
	{name: "gintama", query: "gintama", frags: []string{"gintama"}, episodes: 201, format: "TV", malID: 918},
	{name: "fate_stay", query: "fate stay night", frags: []string{"fate"}, episodes: 0, format: "TV", malID: 22297},
	{name: "rezero", query: "re:zero", frags: []string{"re:zero", "rezero"}, episodes: 25, format: "TV", malID: 31240},
	{name: "steinsgate", query: "steins;gate", frags: []string{"steins"}, episodes: 24, format: "TV", malID: 9253},
	{name: "kon", query: "k-on", frags: []string{"k-on", "k on", "keion"}, episodes: 14, format: "TV", malID: 5680},
	{name: "monogatari", query: "bakemonogatari", frags: []string{"monogatari"}, episodes: 15, format: "TV", malID: 5081},
	{name: "sao_ii", query: "sword art online ii", frags: []string{"sword art online"}, episodes: 24, format: "TV", malID: 21881},
	{name: "aot_romaji", query: "shingeki no kyojin", frags: []string{"attack on titan", "shingeki"}, episodes: 25, format: "TV", malID: 16498},
	{name: "naruto", query: "naruto", frags: []string{"naruto"}, episodes: 220, format: "TV", malID: 20},
	{name: "one_piece", query: "one piece", frags: []string{"one piece"}, episodes: 0, format: "TV", malID: 21},
	{name: "bleach_tybw", query: "bleach thousand year blood war", frags: []string{"bleach"}, episodes: 0, format: "TV"},
	{name: "eva_1", query: "evangelion 1.0", frags: []string{"evangelion"}, episodes: 0, format: "MOVIE", malID: 2759},
	{name: "your_name", query: "your name", frags: []string{"your name", "kimi no na"}, episodes: 0, format: "MOVIE", malID: 32281},
	{name: "kimetsu", query: "kimetsu no yaiba", frags: []string{"demon slayer", "kimetsu"}, episodes: 26, format: "TV", malID: 38000},
	{name: "oshi_no_ko", query: "oshi no ko", frags: []string{"oshi no ko", "my star"}, episodes: 11, format: "TV", malID: 52034},
	{name: "spy_x_family", query: "spy x family", frags: []string{"spy"}, episodes: 25, format: "TV", malID: 50265},
	{name: "eightysix", query: "86", frags: []string{"86"}, episodes: 0, format: "TV"},
	{name: "mushoku", query: "mushoku tensei", frags: []string{"mushoku"}, episodes: 0, format: "TV", malID: 45577},
	{name: "slime_long", query: "that time i got reincarnated as a slime", frags: []string{"slime"}, episodes: 0, format: "TV", malID: 37430},
	{name: "csm", query: "chainsaw man", frags: []string{"chainsaw"}, episodes: 12, format: "TV", malID: 44511},
	{name: "typo_freiren", query: "freiren", frags: []string{"frieren"}, episodes: 0, format: "", noResultOK: true},
	{name: "typo_atack", query: "atack on titan", frags: []string{"attack on titan", "shingeki"}, episodes: 0, format: "", noResultOK: true},
}

type hardVerdict string

const (
	hardOK        hardVerdict = "OK"
	hardWeak      hardVerdict = "WEAK"
	hardNoResult  hardVerdict = "NO-RESULT"
	hardWrong     hardVerdict = "WRONG-PICK"
	hardNoStream  hardVerdict = "NO-STREAM"
	hardTransport hardVerdict = "TRANSPORT-FAIL"
)

type hardResult struct {
	name      string
	verdict   hardVerdict
	detail    string
	searchMs  int64
	streamMs  int64
	providers int
}

func titleMatchesFragments(title string, frags []string) bool {
	lower := strings.ToLower(title)
	for _, frag := range frags {
		if frag != "" && strings.Contains(lower, strings.ToLower(frag)) {
			return true
		}
	}
	return false
}

// probeHardStream fetches a resolved stream URL the way mpv would: browser
// UA (some CDNs reject Go's default) plus the hint referrer.
func probeHardStream(client *http.Client, streamURL, referrer string) error {
	req, err := http.NewRequest(http.MethodGet, streamURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	if referrer != "" {
		req.Header.Set("Referer", referrer)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("playlist status %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "#EXTM3U") {
		return fmt.Errorf("not an HLS playlist")
	}
	return nil
}

func TestHardAnimeBenchmark(t *testing.T) {
	if os.Getenv("CURD_LIVE_HARD") == "" {
		t.Skip("set CURD_LIVE_HARD=1 to run the hard-anime benchmark")
	}
	previous := curdhost.HTTPClient
	curdhost.HTTPClient = func() *http.Client { return http.DefaultClient }
	t.Cleanup(func() { curdhost.HTTPClient = previous })

	// Decline any approval prompts: auto-matches must be silent identity
	// hits, never user-blessed guesses.
	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		return SelectionOption{Key: "manual"}, nil
	})

	config := &CurdConfig{Provider: `["anipm","anikoto","anilight","animex","aniwaves","anipub","senshi"]`, SubOrDub: "sub"}
	results := make([]hardResult, 0, len(hardAnimeCases))

	for _, tc := range hardAnimeCases {
		res := hardResult{name: tc.name}
		start := time.Now()
		animeList, failures, err := searchAnimeStack(config, tc.query, "sub")
		res.searchMs = time.Since(start).Milliseconds()
		res.providers = len(configuredProviderNames(config)) - len(failures)
		if err != nil {
			res.verdict = hardTransport
			res.detail = err.Error()
			results = append(results, res)
			t.Logf("[%s] TRANSPORT-FAIL %s", tc.name, res.detail)
			time.Sleep(time.Second)
			continue
		}
		if len(animeList) == 0 {
			if tc.noResultOK {
				res.verdict = hardWeak
				res.detail = "providers returned nothing usable (custom-rename path)"
			} else {
				res.verdict = hardNoResult
				res.detail = "no results"
			}
			results = append(results, res)
			t.Logf("[%s] %s (%s)", tc.name, res.verdict, res.detail)
			time.Sleep(time.Second)
			continue
		}

		anime := &Anime{
			MalId:         tc.malID,
			TotalEpisodes: tc.episodes,
			Title:         AnimeTitle{English: tc.query, Romaji: tc.query},
		}
		entry := &Entry{Media: Media{Episodes: tc.episodes, Format: tc.format}}
		state := &providerMappingSearchState{query: tc.query}
		if !autoMatchProviderListing(config, anime, animeList, tc.query, entry) {
			ranked := rankProviderSearchOptions(animeList, anime, tc.query, configuredProviderNames(config))
			topTitles := []string{}
			for i, opt := range ranked {
				if i >= 3 {
					break
				}
				topTitles = append(topTitles, opt.Title)
			}
			top := strings.Join(topTitles, " | ")
			if titleMatchesFragments(top, tc.frags) {
				res.verdict = hardWeak
				res.detail = fmt.Sprintf("needs-picker, correct show ranked top-3 of %d: %s", len(animeList), top)
			} else if tc.noResultOK {
				res.verdict = hardWeak
				res.detail = fmt.Sprintf("providers returned only junk (%d results, top: %s); custom-rename path", len(animeList), top)
			} else {
				res.verdict = hardNoResult
				res.detail = fmt.Sprintf("no auto-match among %d results, top: %s", len(animeList), top)
			}
			results = append(results, res)
			t.Logf("[%s] %s %s", tc.name, res.verdict, res.detail)
			_ = state
			time.Sleep(time.Second)
			continue
		}
		applyMatchedProviderMapping(config, state, anime)
		providerName, _ := AnimeProviderID(anime)
		pickedTitle := titleForProviderID(animeList, anime.ProviderId, providerName)
		if !titleMatchesFragments(pickedTitle, tc.frags) {
			res.verdict = hardWrong
			res.detail = fmt.Sprintf("picked %q (%s) for query %q", pickedTitle, anime.ProviderId, tc.query)
			results = append(results, res)
			t.Logf("[%s] WRONG-PICK %s", tc.name, res.detail)
			time.Sleep(time.Second)
			continue
		}
		// Version check: the picked entry must be the right *version*, not
		// just the right franchise. Prefer the provider-reported MAL ID;
		// otherwise compare the provider's episode total.
		pickedOpt := optionForProviderID(animeList, anime.ProviderId, providerName)
		facts := providerFactsFromExtra(pickedOpt.ExtraData)
		if tc.malID != 0 && facts.malID != 0 && facts.malID != tc.malID {
			res.verdict = hardWrong
			res.detail = fmt.Sprintf("picked %q with MAL %d, want %d", pickedTitle, facts.malID, tc.malID)
			results = append(results, res)
			t.Logf("[%s] WRONG-PICK %s", tc.name, res.detail)
			time.Sleep(time.Second)
			continue
		}
		if tc.malID != 0 && facts.malID == 0 && tc.episodes > 0 {
			if eps, err := EpisodesList(QualifyProviderID(providerName, rawProviderKey(anime.ProviderId)), "sub"); err == nil && len(eps) > 0 {
				if total := inferTotalEpisodesFromEpisodeList(eps); total != 0 && total != tc.episodes {
					res.verdict = hardWrong
					res.detail = fmt.Sprintf("picked %q with %d episodes, want %d", pickedTitle, total, tc.episodes)
					results = append(results, res)
					t.Logf("[%s] WRONG-PICK %s", tc.name, res.detail)
					time.Sleep(time.Second)
					continue
				}
			}
		}

		// Real playback path: full preferred-mode stack with identity
		// matching per provider, so a dead mapped provider falls back
		// instead of failing the episode.
		streamStart := time.Now()
		playback, serr := ResolveEpisodeURLForPlayback(CurdConfig{SubOrDub: "sub"}, anime, 1)
		res.streamMs = time.Since(streamStart).Milliseconds()
		if serr != nil || len(playback.Links) == 0 {
			res.verdict = hardNoStream
			res.detail = fmt.Sprintf("stack ep1: %v", serr)
			results = append(results, res)
			t.Logf("[%s] NO-STREAM %s", tc.name, res.detail)
			time.Sleep(time.Second)
			continue
		}
		providerName = playback.ProviderName
		if perr := probeHardStream(http.DefaultClient, playback.Links[0], playback.LinkHints[playback.Links[0]].Referrer); perr != nil {
			res.verdict = hardNoStream
			res.detail = fmt.Sprintf("%s playlist: %v", providerName, perr)
			results = append(results, res)
			t.Logf("[%s] NO-STREAM %s", tc.name, res.detail)
			time.Sleep(time.Second)
			continue
		}
		res.verdict = hardOK
		res.detail = fmt.Sprintf("picked %q via %s (%dms+%dms)", pickedTitle, providerName, res.searchMs, res.streamMs)
		results = append(results, res)
		t.Logf("[%s] OK %s", tc.name, res.detail)
		time.Sleep(time.Second)
	}

	counts := map[hardVerdict]int{}
	var totalSearch, totalStream int64
	var streams int
	for _, res := range results {
		counts[res.verdict]++
		totalSearch += res.searchMs
		if res.verdict == hardOK {
			totalStream += res.streamMs
			streams++
		}
	}
	t.Logf("=== hard benchmark: %d cases ===", len(results))
	for _, v := range []hardVerdict{hardOK, hardWeak, hardNoResult, hardWrong, hardNoStream, hardTransport} {
		t.Logf("  %s: %d", v, counts[v])
	}
	if len(results) > 0 {
		avgStream := int64(0)
		if streams > 0 {
			avgStream = totalStream / int64(streams)
		}
		t.Logf("  avg search=%dms avg stream=%dms", totalSearch/int64(len(results)), avgStream)
	}
	for _, res := range results {
		if res.verdict == hardWrong || res.verdict == hardNoStream || (res.verdict == hardNoResult) {
			t.Errorf("[%s] %s: %s", res.name, res.verdict, res.detail)
		}
	}
}

func firstTitle(options []SelectionOption) string {
	if len(options) == 0 {
		return ""
	}
	return options[0].Title
}

func titleForProviderID(options []SelectionOption, providerID, providerName string) string {
	return optionForProviderID(options, providerID, providerName).Title
}

func optionForProviderID(options []SelectionOption, providerID, providerName string) SelectionOption {
	for _, option := range options {
		name, raw, ok := ParseProviderQualifiedID(option.Key)
		if ok && name == providerName && raw == providerID {
			return option
		}
		if !ok && option.Key == providerID {
			return option
		}
	}
	return SelectionOption{}
}
