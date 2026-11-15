package internal

// Headless end-to-end tests for the whole application flow.
//
// These tests exercise the path a user takes in the terminal UI — configure
// providers, search, pick a show, list episodes, resolve a playable link,
// remember progress — without a terminal, rofi, mpv, a browser, or the
// network. Providers are replaced through the registry's test seam
// (providers.SetFactoryForTest), menus through the promptSelect seam, and
// storage through t.TempDir files. If any step panics, the test fails:
// "graceful" here means an error the UI could show, not a crash.
//
// Run with:
//
//	go test ./internal -run TestHeadless -v

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/providers"
)

// e2eStubProvider is a programmable providers.Provider for headless tests.
type e2eStubProvider struct {
	name       string
	searchOpts []providers.SelectionOption
	searchErr  error
	episodes   []string
	epErr      error
	links      []string
	linkErr    error
	calls      []string
}

func (s *e2eStubProvider) Name() string { return s.name }

func (s *e2eStubProvider) SearchAnime(query, mode string) ([]providers.SelectionOption, error) {
	s.calls = append(s.calls, "search:"+query+":"+mode)
	if s.searchErr != nil {
		return nil, s.searchErr
	}
	return s.searchOpts, nil
}

func (s *e2eStubProvider) EpisodesList(showID, mode string) ([]string, error) {
	s.calls = append(s.calls, "episodes:"+showID+":"+mode)
	if s.epErr != nil {
		return nil, s.epErr
	}
	if showID == "" {
		return nil, fmt.Errorf("empty show id")
	}
	return s.episodes, nil
}

func (s *e2eStubProvider) GetEpisodeURL(config providers.PlaybackConfig, id string, epNo int) ([]string, error) {
	return s.GetEpisodeURLForMode(config, id, epNo, config.SubOrDub)
}

func (s *e2eStubProvider) GetEpisodeURLForMode(config providers.PlaybackConfig, id string, epNo int, mode string) ([]string, error) {
	links, _, err := s.GetEpisodeURLForModeWithHints(config, id, epNo, mode)
	return links, err
}

func (s *e2eStubProvider) GetEpisodeURLForModeWithHints(config providers.PlaybackConfig, id string, epNo int, mode string) ([]string, map[string]providers.StreamPlaybackHint, error) {
	s.calls = append(s.calls, fmt.Sprintf("stream:%s:%d:%s", id, epNo, mode))
	if s.linkErr != nil {
		return nil, nil, s.linkErr
	}
	if epNo <= 0 || epNo > len(s.episodes) {
		return nil, nil, fmt.Errorf("episode %d out of range", epNo)
	}
	hints := make(map[string]providers.StreamPlaybackHint, len(s.links))
	for _, link := range s.links {
		hints[link] = providers.StreamPlaybackHint{Referrer: "https://example.test/"}
	}
	return s.links, hints, nil
}

func installE2EStubs(t *testing.T, stubs ...*e2eStubProvider) {
	t.Helper()
	withAllProvidersEnabledForTest(t)
	restores := make([]func(), 0, len(stubs))
	for _, stub := range stubs {
		stub := stub
		restores = append(restores, providers.SetFactoryForTest(stub.name, func() providers.Provider {
			return stub
		}))
	}
	t.Cleanup(func() {
		for i := len(restores) - 1; i >= 0; i-- {
			restores[i]()
		}
	})
}

func installE2EConfig(t *testing.T, config *CurdConfig) {
	t.Helper()
	previous := GetGlobalConfig()
	previousPath := GlobalConfigPath
	previousProvider := CurrentProvider
	SetGlobalConfig(config)
	GlobalConfigPath = filepath.Join(t.TempDir(), "curd.conf")
	CurrentProvider = nil
	t.Cleanup(func() {
		SetGlobalConfig(previous)
		GlobalConfigPath = previousPath
		CurrentProvider = previousProvider
	})
}

// TestHeadlessSearchToPlayableLink is the central flow: the primary provider
// is down, the stack falls back, episodes resolve, and the link the player
// would open is chosen — all without a GUI.
func TestHeadlessSearchToPlayableLink(t *testing.T) {
	down := &e2eStubProvider{name: "senshi", searchErr: fmt.Errorf("senshi is down"), linkErr: fmt.Errorf("senshi is down")}
	good := &e2eStubProvider{
		name:       "anipub",
		searchOpts: []providers.SelectionOption{{Key: "2454", Label: "Frieren: Beyond Journey's End", Title: "Frieren: Beyond Journey's End"}},
		episodes:   []string{"1", "2"},
		links:      []string{"https://cdn.example/720p.m3u8", "https://video.wixstatic.com/1080p.m3u8"},
	}
	installE2EStubs(t, down, good)
	installE2EConfig(t, &CurdConfig{Provider: `["senshi","anipub"]`, SubOrDub: "sub"})

	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		t.Fatalf("no menu should be shown in the headless happy path, got %#v", options)
		return SelectionOption{}, nil
	})

	results, err := SearchAnime("frieren", "sub")
	if err != nil {
		t.Fatalf("SearchAnime: %v", err)
	}
	if len(results) != 1 || results[0].Key != "anipub::2454" {
		t.Fatalf("expected one qualified anipub result, got %#v", results)
	}

	episodes, err := EpisodesList(results[0].Key, "sub")
	if err != nil {
		t.Fatalf("EpisodesList: %v", err)
	}
	if strings.Join(episodes, ",") != "1,2" {
		t.Fatalf("unexpected episodes %v", episodes)
	}

	links, mode, err := GetEpisodeURLForPlayback(CurdConfig{SubOrDub: "sub"}, results[0].Key, 1)
	if err != nil {
		t.Fatalf("GetEpisodeURLForPlayback: %v", err)
	}
	if mode != "sub" || len(links) == 0 {
		t.Fatalf("unexpected links/mode: %v %q", links, mode)
	}
	if picked := PrioritizeLink(links); picked != "https://video.wixstatic.com/1080p.m3u8" {
		t.Fatalf("player would open %q, want the prioritized 1080p link", picked)
	}

	// Local history round-trip: progress survives a restart (file reload).
	dbFile := filepath.Join(t.TempDir(), "curd_history.txt")
	if err := LocalUpdateAnime(dbFile, 52991, results[0].Key, 1, 0, 0, "Frieren: Beyond Journey's End", "anipub"); err != nil {
		t.Fatalf("LocalUpdateAnime: %v", err)
	}
	history := LocalGetAllAnime(dbFile)
	if len(history) != 1 || history[0].AnilistId != 52991 || history[0].Ep.Number != 1 {
		t.Fatalf("history did not round-trip: %+v", history)
	}
	if err := LocalUpdateAnime(dbFile, 52991, results[0].Key, 2, 0, 0, "Frieren: Beyond Journey's End", "anipub"); err != nil {
		t.Fatalf("LocalUpdateAnime update: %v", err)
	}
	history = LocalGetAllAnime(dbFile)
	if len(history) != 1 || history[0].Ep.Number != 2 {
		t.Fatalf("history update did not round-trip: %+v", history)
	}

	// Config round-trip through the real file format used by main().
	configPath := filepath.Join(t.TempDir(), "curd.conf")
	if err := SaveConfigToFile(configPath, map[string]string{"Provider": `["anipub"]`, "SubOrDub": "sub"}); err != nil {
		t.Fatalf("SaveConfigToFile: %v", err)
	}
	loaded, err := LoadConfigFromFile(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFromFile: %v", err)
	}
	names, _ := parseProviderConfig(loaded["Provider"])
	if len(names) != 1 || names[0] != "anipub" {
		t.Fatalf("reloaded provider config = %v, want [anipub]", names)
	}
}

// TestHeadlessAllProvidersDown proves the stack degrades to one clean error
// (the thing the UI shows) instead of a panic or a hang when every provider
// fails at every step.
func TestHeadlessAllProvidersDown(t *testing.T) {
	boom := fmt.Errorf("upstream exploded")
	installE2EStubs(t,
		&e2eStubProvider{name: "senshi", searchErr: boom, epErr: boom, linkErr: boom},
		&e2eStubProvider{name: "anipub", searchErr: boom, epErr: boom, linkErr: boom},
	)
	installE2EConfig(t, &CurdConfig{Provider: `["senshi","anipub"]`, SubOrDub: "sub"})
	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		return SelectionOption{Key: "cancel"}, nil
	})

	if _, err := SearchAnime("frieren", "sub"); err == nil {
		t.Fatal("SearchAnime with all providers down must return an error")
	}
	if _, err := EpisodesList("senshi::52991", "sub"); err == nil {
		t.Fatal("EpisodesList with provider down must return an error")
	}
	if links, _, err := GetEpisodeURLForPlayback(CurdConfig{SubOrDub: "sub"}, "senshi::52991", 1); err == nil || len(links) > 0 {
		t.Fatalf("GetEpisodeURLForPlayback with provider down = %v, %v; want error and no links", links, err)
	}
}

// TestHeadlessEdgeCasesNoPanic sweeps malformed inputs through the parsing
// and storage boundaries the UI feeds from config files, provider responses,
// and the local history file. Every call must return normally.
func TestHeadlessEdgeCasesNoPanic(t *testing.T) {
	weird := []string{
		"", " ", "[]", "[,", `["senshi",`, "no-such-provider", "senshi::",
		"::52991", "senshi::52991::extra", strings.Repeat("x", 4096),
		"💥", "\x00\x01", "senshi,💥,anipub",
	}
	for _, raw := range weird {
		names, _ := parseProviderConfig(raw)
		for _, name := range names {
			if providers.NormalizeName(name) == "" {
				t.Fatalf("parseProviderConfig(%q) returned unregistered name %q", raw, name)
			}
		}
		_, _, _ = ParseProviderQualifiedID(raw)
		_ = QualifyProviderID("senshi", raw)
		_ = QualifyProviderID(raw, raw)
	}

	if got := PrioritizeLink(nil); got != "" {
		t.Fatalf("PrioritizeLink(nil) = %q, want empty", got)
	}
	if got := PrioritizeLink([]string{"", "  "}); got != "" && got != "  " {
		t.Fatalf("PrioritizeLink(blank) = %q", got)
	}
	dup := PrioritizeLink([]string{"https://cdn.example/a.m3u8", "https://cdn.example/a.m3u8"})
	if dup != "https://cdn.example/a.m3u8" {
		t.Fatalf("PrioritizeLink(dup) = %q", dup)
	}

	if total := inferTotalEpisodesFromEpisodeList([]string{"", "abc", "-1", "0", "999999999999999999999999", "1.5", "12"}); total != 12 {
		t.Fatalf("inferTotalEpisodesFromEpisodeList = %d, want 12", total)
	}

	// Corrupt / truncated history files must load as empty or partial, never crash.
	dir := t.TempDir()
	corrupt := filepath.Join(dir, "curd_history.txt")
	if err := os.WriteFile(corrupt, []byte("not,a,valid\n\x00\x01\x02\n1,2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_ = LocalGetAllAnime(corrupt)
	_ = LocalGetAllAnime(filepath.Join(dir, "missing", "history.txt"))
	if err := LocalUpdateAnime(corrupt, 0, "", -1, -5, -9, "", ""); err != nil {
		// An error is acceptable; a panic would fail the test.
		t.Logf("LocalUpdateAnime on corrupt file returned error (acceptable): %v", err)
	}

	// Unknown and disabled-provider lookups must error, not panic.
	withAllProvidersEnabledForTest(t)
	if _, err := ProviderByName("no-such-provider"); err == nil {
		t.Fatal("ProviderByName(unknown) must error")
	}
	if _, err := ProviderByName(""); err == nil {
		t.Fatal("ProviderByName(empty) must error")
	}
}
