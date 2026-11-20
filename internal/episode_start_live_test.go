package internal

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLiveEpisodeStartE2E runs the real "user picked a show → video is
// playing" path for one episode, against the user's real config but a cold,
// isolated cache: provider search across the stack, identity auto-match,
// the time-boxed Jikan check, stream resolve with hints, and the app's own
// StartVideo launching real mpv (headless). Passes only when mpv's playback
// position actually advances — a resolved URL is not proof of playback.
//
//	CURD_LIVE_E2E=1 go test ./internal -run TestLiveEpisodeStartE2E -v -timeout 5m
//
// Optional: CURD_E2E_MODE=dub, CURD_E2E_EP=<n>.
func TestLiveEpisodeStartE2E(t *testing.T) {
	if os.Getenv("CURD_LIVE_E2E") != "1" {
		t.Skip("set CURD_LIVE_E2E=1")
	}
	home, _ := os.UserHomeDir()
	cfg, err := LoadConfig(filepath.Join(home, ".config", "curd", "curd.conf"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.StoragePath = t.TempDir() // cold caches: worst case for a new episode
	cfg.MpvArgs = nil
	if m := os.Getenv("CURD_E2E_MODE"); m != "" {
		cfg.SubOrDub = m
	}
	SetGlobalConfig(&cfg)
	user := User{}
	SetGlobalUser(&user)

	ep := 5
	if v := os.Getenv("CURD_E2E_EP"); v != "" {
		if n, err := parseNonNegativeIntInput(v, "ep"); err == nil && n > 0 {
			ep = n
		}
	}
	anime := &Anime{
		AnilistId:     14719,
		MalId:         14719,
		TotalEpisodes: 26,
		Title:         AnimeTitle{Romaji: "JoJo no Kimyou na Bouken (TV)", English: "JoJo's Bizarre Adventure"},
	}
	anime.Ep.Number = ep
	SetGlobalAnime(anime)
	mode := normalizeTranslationType(cfg.SubOrDub)

	total := time.Now()
	mark := func(name string, start time.Time) time.Duration {
		d := time.Since(start)
		t.Logf("%-28s %8s", name, d.Round(time.Millisecond))
		return d
	}

	s := time.Now()
	options, err := SearchAnime(anime.Title.Romaji, mode)
	mark("search (stacked)", s)
	if err != nil || len(options) == 0 {
		t.Fatalf("search: %d results, err %v", len(options), err)
	}

	s = time.Now()
	if !autoMatchProviderListing(&cfg, anime, options, anime.Title.Romaji, nil) {
		t.Fatalf("no confident auto-match among %d results", len(options))
	}
	mark("auto-match", s)
	t.Logf("matched provider id: %s", anime.ProviderId)

	s = time.Now()
	GetEpisodeDataWithin(anime.MalId, ep, anime, jikanPrePlayBudget)
	if d := mark("pre-play metadata", s); d > jikanPrePlayBudget+300*time.Millisecond {
		t.Errorf("pre-play metadata blew its budget: %s", d)
	}

	s = time.Now()
	res, err := ResolveEpisodeURLForPlayback(cfg, anime, ep)
	mark("resolve stream", s)
	if err != nil || len(res.Links) == 0 {
		t.Fatalf("resolve ep %d %s: %v", ep, mode, err)
	}
	if res.Mode != mode {
		t.Fatalf("asked for %s, resolver returned %s (silent mode switch)", mode, res.Mode)
	}
	anime.Ep.Links = res.Links
	applyStreamPlaybackHints(anime, res.Links, res.LinkHints)
	t.Logf("provider=%s mode=%s referrer=%q subtitle=%v", res.ProviderName, res.Mode, anime.Ep.StreamReferrer, anime.Ep.SubtitleURL != "")

	s = time.Now()
	link := PrioritizeLink(anime.Ep.Links)
	sock, err := StartVideo(link, []string{"--vo=null", "--ao=null", "--no-terminal"}, "e2e", anime)
	if err != nil {
		t.Fatalf("StartVideo: %v", err)
	}
	defer func() { _, _ = MPVSendCommand(sock, []interface{}{"quit"}) }()

	deadline := time.Now().Add(60 * time.Second)
	var pos float64
	for time.Now().Before(deadline) {
		if v, err := MPVSendCommand(sock, []interface{}{"get_property", "time-pos"}); err == nil {
			if f, ok := v.(float64); ok && f > 1.0 {
				pos = f
				break
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	mark("mpv first second played", s)
	if pos <= 1.0 {
		t.Fatal("mpv never advanced playback: stream is not actually playable")
	}
	if v, err := MPVSendCommand(sock, []interface{}{"get_property", "duration"}); err == nil {
		t.Logf("duration reported by mpv: %v", v)
	}
	if d := mark("TOTAL pick → playing", total); d > 20*time.Second {
		t.Errorf("episode start took %s", d)
	}
}
