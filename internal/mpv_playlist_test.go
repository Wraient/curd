package internal

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestEpisodeLabelMarksCurrentAndFiller(t *testing.T) {
	c := &MPVPlaylistController{
		anime: &Anime{
			Title:          AnimeTitle{English: "Demo"},
			FillerEpisodes: []int{3, 5},
			Ep:             Episode{Number: 2},
		},
		preferredMode:  "sub",
		currentPlaying: 2,
		currentMode:    "sub",
	}
	label := c.episodeLabel(2, "sub")
	if !strings.Contains(label, "Episode 2") || !strings.Contains(label, "◀") {
		t.Fatalf("current ep label: %q", label)
	}
	if !strings.Contains(label, "Demo") {
		t.Fatalf("expected show name in label: %q", label)
	}
	filler := c.episodeLabel(3, "sub")
	if !strings.Contains(filler, "Filler") {
		t.Fatalf("expected filler mark, got %q", filler)
	}
	dub := c.episodeLabel(2, "dub")
	if !strings.Contains(strings.ToUpper(dub), "DUB") {
		t.Fatalf("expected dub mark, got %q", dub)
	}
	// Preferred dub should not spam DUB on every preferred row.
	c.preferredMode = "dub"
	c.currentMode = "dub"
	preferredDub := c.episodeLabel(1, "dub")
	if strings.Contains(preferredDub, "(DUB)") {
		t.Fatalf("preferred dub row should not add (DUB): %q", preferredDub)
	}
}

func TestIsEpisodeFillerHelper(t *testing.T) {
	if !IsEpisodeFiller([]int{1, 4, 7}, 4) {
		t.Fatal("expected filler")
	}
	if IsEpisodeFiller([]int{1, 4, 7}, 3) {
		t.Fatal("expected non-filler")
	}
}

func TestParseEpisodeNumberListDedupesAndSorts(t *testing.T) {
	got := parseEpisodeNumberList([]string{"3", "1", "2", "2", " 12.0 ", "bad", "0", "-1"})
	want := []int{1, 2, 3, 12}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestWritePlaceholderM3UHasEXTINFTitles(t *testing.T) {
	c := &MPVPlaylistController{
		config: &CurdConfig{StoragePath: t.TempDir()},
		anime:  &Anime{Title: AnimeTitle{English: "Demo"}},
	}
	slots := []playlistSlot{
		{Episode: 1, Mode: "sub", Label: "Demo - Episode 1"},
		{Episode: 2, Mode: "sub", Label: "Demo - Episode 2 [Filler]"},
	}
	path, err := c.writePlaceholderM3U(slots)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.HasPrefix(body, "#EXTM3U\n") {
		t.Fatalf("missing EXTM3U header: %q", body)
	}
	if !strings.Contains(body, "#EXTINF:-1,Demo - Episode 1\n") {
		t.Fatalf("missing titled entry 1: %q", body)
	}
	if !strings.Contains(body, "#EXTINF:-1,Demo - Episode 2 [Filler]\n") {
		t.Fatalf("missing titled entry 2: %q", body)
	}
	if !strings.Contains(body, placeholderURL(1)) || !strings.Contains(body, placeholderURL(2)) {
		t.Fatalf("missing unique placeholder urls: %q", body)
	}
	// Must not leave bare lavfi as the only human-visible line without EXTINF.
	lines := strings.Split(strings.TrimSpace(body), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "av://") && i > 0 && !strings.HasPrefix(lines[i-1], "#EXTINF:") {
			t.Fatalf("placeholder without preceding EXTINF at line %d", i)
		}
	}
}

func TestSanitizeM3UTitleStripsNewlines(t *testing.T) {
	got := sanitizeM3UTitle("Foo\nBar\rBaz")
	if strings.ContainsAny(got, "\n\r") {
		t.Fatalf("newlines remain: %q", got)
	}
	if got == "" {
		t.Fatal("empty after sanitize")
	}
}

func TestMPVPlaylistSwitchingFlag(t *testing.T) {
	endMPVPlaylistSwitch()
	if MPVPlaylistIsSwitching() {
		t.Fatal("expected switch flag clear")
	}
	beginMPVPlaylistSwitch()
	if !MPVPlaylistIsSwitching() {
		t.Fatal("expected switch flag set")
	}
	endMPVPlaylistSwitch()
	if MPVPlaylistIsSwitching() {
		t.Fatal("expected switch flag clear after end")
	}
}

func TestClassifyPlaybackLossDoesNotExitWhileMPVAlive(t *testing.T) {
	endMPVPlaylistSwitch()
	// Incomplete watch + empty socket (IsMPVRunning false) → exit
	if got := ClassifyPlaybackLoss("", true, 11, 85); got != PlaybackLossExit {
		t.Fatalf("empty socket incomplete: got %d want Exit", got)
	}
	// Complete threshold → complete regardless of socket
	if got := ClassifyPlaybackLoss("", true, 90, 85); got != PlaybackLossComplete {
		t.Fatalf("complete: got %d want Complete", got)
	}
	// Switching flag forces wait
	beginMPVPlaylistSwitch()
	if got := ClassifyPlaybackLoss("", true, 11, 85); got != PlaybackLossWait {
		t.Fatalf("switching: got %d want Wait", got)
	}
	endMPVPlaylistSwitch()
	// Not started → wait
	if got := ClassifyPlaybackLoss("", false, 0, 85); got != PlaybackLossWait {
		t.Fatalf("not started: got %d want Wait", got)
	}
}

func TestPlaceholderIsLongLivedAndUniquePerEpisode(t *testing.T) {
	a := placeholderURL(39)
	b := placeholderURL(40)
	if a == b {
		t.Fatal("placeholder URLs must differ per episode")
	}
	if !strings.Contains(a, "d=86400") {
		t.Fatalf("placeholder should be long-lived: %q", a)
	}
	// Fragments break lavfi in mpv — ensure we never emit them.
	if strings.Contains(a, "#") {
		t.Fatalf("must not use URL fragments: %q", a)
	}
	for _, ep := range []int{1, 39, 40, 366, 1000} {
		url := placeholderURL(ep)
		got, ok := parseEpisodeFromPlaceholder(url)
		if !ok || got != ep {
			t.Fatalf("ep %d: url=%q parsed=%d ok=%v", ep, url, got, ok)
		}
	}
}

func TestResolvePlaylistLeaveDefaultNeverRegressesOnRewind(t *testing.T) {
	// Backward jump → never touch AniList/MAL, regardless of watch percentage.
	if action := resolvePlaylistLeaveDefault(40, 39, 11, 85); action != playlistLeaveNone {
		t.Fatalf("backward 11%% → %s want none", action)
	}
	if action := resolvePlaylistLeaveDefault(40, 39, 90, 85); action != playlistLeaveNone {
		t.Fatalf("backward 90%% → %s want none", action)
	}
	if action := resolvePlaylistLeaveDefault(40, 1, 0, 85); action != playlistLeaveNone {
		t.Fatalf("backward 0%% → %s want none", action)
	}
}

func TestResolvePlaylistLeaveDefaultNeverPrompts(t *testing.T) {
	// Same episode (mode toggle) → none.
	if action := resolvePlaylistLeaveDefault(40, 40, 90, 85); action != playlistLeaveNone {
		t.Fatalf("same ep → %s want none", action)
	}
	// Next +1 follows standard auto-mark behavior.
	if action := resolvePlaylistLeaveDefault(40, 41, 11, 85); action != playlistLeaveNone {
		t.Fatalf("forward early → %s want none", action)
	}
	if action := resolvePlaylistLeaveDefault(40, 41, 90, 85); action != playlistLeaveMarkLeft {
		t.Fatalf("forward nearly-done → %s want mark", action)
	}
	// Forward skip (>+1) → none (deferred to session end).
	if action := resolvePlaylistLeaveDefault(40, 45, 90, 85); action != playlistLeaveNone {
		t.Fatalf("forward skip → %s want none", action)
	}
}

func TestParsePlaylistEpisodeTitle(t *testing.T) {
	ep, mode, ok := parsePlaylistEpisodeTitle("Bleach - Episode 39", "sub")
	if !ok || ep != 39 || mode != "sub" {
		t.Fatalf("got ep=%d mode=%s ok=%v", ep, mode, ok)
	}
	ep, mode, ok = parsePlaylistEpisodeTitle("Bleach - Episode 40 (DUB)", "sub")
	if !ok || ep != 40 || mode != "dub" {
		t.Fatalf("dub got ep=%d mode=%s ok=%v", ep, mode, ok)
	}
	_, _, ok = parsePlaylistEpisodeTitle("av://lavfi:color", "sub")
	if ok {
		t.Fatal("lavfi should not parse")
	}
}

func TestMediaURLKeyStripsQuery(t *testing.T) {
	a := mediaURLKey("https://cdn.example/x/playlist.m3u8?token=abc")
	b := mediaURLKey("https://cdn.example/x/playlist.m3u8?token=xyz")
	if a != b {
		t.Fatalf("keys should match without query: %q vs %q", a, b)
	}
	if mediaURLKey("https://cdn.example/a/p.m3u8") == mediaURLKey("https://cdn.example/b/p.m3u8") {
		t.Fatal("different paths should differ")
	}
}

func TestEscapeMPVOptionValueQuotes(t *testing.T) {
	got := escapeMPVOptionValue(`Show - Episode 1`)
	if !strings.HasPrefix(got, `"`) || !strings.HasSuffix(got, `"`) {
		t.Fatalf("expected quoted: %q", got)
	}
	got = escapeMPVOptionValue(`a"b`)
	if !strings.Contains(got, `\"`) {
		t.Fatalf("expected escaped quote: %q", got)
	}
}

func TestFinalizePlaylistEpisodeChangeUpdatesLocalHistory(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "curd_history.txt")
	// anikoto is NoPrefetch: the fire-and-forget next-episode prefetch
	// spawned by finalizePlaylistEpisodeChange exits before any network,
	// so it cannot leak into later tests and race their HTTP hooks.
	// These tests assert history/filler bookkeeping, not prefetch results.
	anime := &Anime{
		AnilistId:      269,
		ProviderId:     "269",
		ProviderName:   "anikoto",
		Title:          AnimeTitle{English: "Bleach"},
		FillerEpisodes: []int{33},
		Ep:             Episode{Number: 40},
	}
	c := &MPVPlaylistController{
		config:        &CurdConfig{StoragePath: dir, SubOrDub: "sub"},
		anime:         anime,
		preferredMode: "sub",
		socket:        "", // skip MPV title commands failures are fine
	}

	c.finalizePlaylistEpisodeChange(40, 39, "sub", playlistLeaveNone)

	if anime.Ep.Number != 39 {
		t.Fatalf("episode number=%d want 39", anime.Ep.Number)
	}
	if anime.Ep.IsFiller {
		t.Fatal("ep 39 should not be filler")
	}
	// curd_id written
	idRaw, err := os.ReadFile(filepath.Join(dir, "curd_id"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(idRaw)) != "269" {
		t.Fatalf("curd_id=%q", idRaw)
	}
	// history row
	list := LocalGetAllAnime(db)
	found := LocalFindAnime(list, 269, "269")
	if found == nil {
		t.Fatal("expected history entry")
	}
	if found.Ep.Number != 39 {
		t.Fatalf("history ep=%d want 39", found.Ep.Number)
	}
	if found.Ep.Player.PlaybackTime != 0 {
		t.Fatalf("playback time should reset, got %d", found.Ep.Player.PlaybackTime)
	}
}

func TestFinalizePlaylistMarksFillerFromList(t *testing.T) {
	dir := t.TempDir()
	// anikoto is NoPrefetch: see above — these tests assert filler
	// bookkeeping, and must not leak background network into later tests.
	anime := &Anime{
		AnilistId:      1,
		ProviderId:     "x",
		ProviderName:   "anikoto",
		Title:          AnimeTitle{English: "Demo"},
		FillerEpisodes: []int{5},
		Ep:             Episode{Number: 1},
	}
	c := &MPVPlaylistController{
		config: &CurdConfig{StoragePath: dir},
		anime:  anime,
	}
	c.finalizePlaylistEpisodeChange(1, 5, "sub", playlistLeaveNone)
	if !anime.Ep.IsFiller {
		t.Fatal("expected filler flag for ep 5")
	}
	// silence unused if LocalGetAllAnime not needed
	_ = strconv.Itoa(anime.Ep.Number)
}

func TestLocalHistoryPath(t *testing.T) {
	if localHistoryPath("") != "" {
		t.Fatal("empty storage")
	}
	got := localHistoryPath("/tmp/curd-test-store")
	if !strings.HasSuffix(got, "curd_history.txt") {
		t.Fatalf("got %q", got)
	}
}
