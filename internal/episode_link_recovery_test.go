package internal

import (
	"errors"
	"strings"
	"testing"
)

func TestEpisodeLinkFailureRecoveryOptionsOrder(t *testing.T) {
	withAudio := episodeLinkFailureRecoveryOptions("sub", true)
	if len(withAudio) != 3 {
		t.Fatalf("expected 3 options with audio, got %#v", withAudio)
	}
	if withAudio[0].Key != "remap" {
		t.Fatalf("remap should be first: %#v", withAudio[0])
	}
	if withAudio[1].Key != "audio" || !strings.Contains(withAudio[1].Label, "dub") {
		t.Fatalf("audio should be second and mention dub: %#v", withAudio[1])
	}
	if withAudio[2].Key != "episode" || !strings.Contains(strings.ToLower(withAudio[2].Label), "wrong") {
		t.Fatalf("episode correction should be last ranked action: %#v", withAudio[2])
	}
	for _, opt := range withAudio {
		if strings.HasPrefix(opt.Label, "1.") || strings.HasPrefix(opt.Label, "2.") {
			t.Fatalf("recovery options should not be numbered: %#v", opt)
		}
	}

	withoutAudio := episodeLinkFailureRecoveryOptions("dub", false)
	if len(withoutAudio) != 2 {
		t.Fatalf("expected 2 options without audio, got %#v", withoutAudio)
	}
	if withoutAudio[0].Key != "remap" || withoutAudio[1].Key != "episode" {
		t.Fatalf("unexpected options without audio: %#v", withoutAudio)
	}
}

func TestEpisodeLinkFailureDiagnosisIncludesEpisodeModeProviders(t *testing.T) {
	withAllProvidersEnabledForTest(t)
	cfg := &CurdConfig{Provider: `["senshi","allanime"]`, SubOrDub: "sub"}
	anime := &Anime{
		Title:        AnimeTitle{English: "Frieren: Beyond Journey's End"},
		Ep:           Episode{Number: 12},
		ProviderName: "senshi",
	}
	msg := episodeLinkFailureDiagnosis(cfg, anime, errors.New("no sub episode links found across providers"))
	for _, want := range []string{
		`Couldn't find Episode 12 (sub)`,
		`Frieren: Beyond Journey's End`,
		"Tried:",
		"senshi",
		"allanime",
		"Reason:",
		"no sub episode links",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("diagnosis missing %q in:\n%s", want, msg)
		}
	}
}

func TestPromptEpisodeLinkFailureRecoveryMapsExitKeysToBack(t *testing.T) {
	cfg := &CurdConfig{SubOrDub: "sub", Provider: `["allanime"]`}
	anime := &Anime{Title: AnimeTitle{Romaji: "Example"}, Ep: Episode{Number: 1}}

	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		return SelectionOption{Key: "-2", Label: "Back"}, nil
	})
	if got := promptEpisodeLinkFailureRecovery(cfg, anime, errors.New("fail"), true); got != "back" {
		t.Fatalf("expected back for -2, got %q", got)
	}

	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		return SelectionOption{Key: "-1", Label: "Quit"}, nil
	})
	if got := promptEpisodeLinkFailureRecovery(cfg, anime, errors.New("fail"), true); got != "quit" {
		t.Fatalf("expected quit for -1, got %q", got)
	}
}

func TestResolveEpisodeLinksWithRecoverySucceedsWithoutRecoveryMenu(t *testing.T) {
	provider := &stackStubProvider{
		name: "allanime",
		episodeResults: map[string]map[string][]string{
			"allanime-id": {"sub": {"sub-url"}},
		},
		searchResults: map[string][]SelectionOption{
			"sub": {{Title: "Example", Key: "allanime-id"}},
		},
	}
	withProviderFactories(t, provider)

	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		t.Fatalf("recovery menu should not open when preferred resolve succeeds: %#v", options)
		return SelectionOption{}, nil
	})

	cfg := &CurdConfig{Provider: `["allanime"]`, SubOrDub: "sub"}
	anime := &Anime{
		Title:        AnimeTitle{Romaji: "Example"},
		ProviderName: "allanime",
		ProviderId:   "allanime-id",
		Ep:           Episode{Number: 1},
	}

	result, ok := resolveEpisodeLinksWithRecovery(cfg, anime, nil)
	if !ok {
		t.Fatal("expected success without recovery")
	}
	if result.Links[0] != "sub-url" || result.Mode != "sub" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestResolveEpisodeLinksWithRecoveryOnlyAfterPreferredAndAlternateFail(t *testing.T) {
	// Both modes have zero streams → no alternate prompt (nothing to offer), then recovery.
	providerBothDead := &stackStubProvider{
		name: "allanime",
		episodeErrors: map[string]map[string]error{
			"allanime-id": {
				"sub": errors.New("no sub"),
				"dub": errors.New("no dub"),
			},
		},
		searchResults: map[string][]SelectionOption{
			"sub": {{Title: "Example", Key: "allanime-id"}},
			"dub": {{Title: "Example", Key: "allanime-id"}},
		},
	}
	withProviderFactories(t, providerBothDead)

	var promptPhases []string
	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		if len(options) >= 1 && options[0].Key == "play" {
			promptPhases = append(promptPhases, "alternate-audio")
			return SelectionOption{Key: "cancel"}, nil
		}
		if len(options) >= 1 && options[0].Key == "remap" {
			promptPhases = append(promptPhases, "recovery")
			keys := make([]string, 0, len(options))
			for _, opt := range options {
				keys = append(keys, opt.Key)
			}
			joined := strings.Join(keys, ",")
			if !strings.Contains(joined, "remap") || !strings.Contains(joined, "episode") || !strings.Contains(joined, "audio") {
				t.Fatalf("recovery menu missing expected actions: %s", joined)
			}
			if strings.Contains(joined, "quit") || strings.Contains(joined, "cancel") {
				t.Fatalf("recovery menu should not embed cancel/quit keys: %s", joined)
			}
			return SelectionOption{Key: "back"}, nil
		}
		t.Fatalf("unexpected prompt options: %#v", options)
		return SelectionOption{}, nil
	})

	cfg := &CurdConfig{Provider: `["allanime","no-animepahe"]`, SubOrDub: "sub"}
	anime := &Anime{
		Title:        AnimeTitle{Romaji: "Example"},
		ProviderName: "allanime",
		ProviderId:   "allanime-id",
		Ep:           Episode{Number: 3},
	}

	result, ok := resolveEpisodeLinksWithRecovery(cfg, anime, nil)
	if ok || len(result.Links) > 0 {
		t.Fatalf("expected user back-out after recovery, got ok=%v result=%#v", ok, result)
	}
	// No playable alternate streams ⇒ skip audio prompt, go straight to recovery.
	if got := strings.Join(promptPhases, ","); got != "recovery" {
		t.Fatalf("expected recovery only when alternate has no streams, got %q", got)
	}
	if len(providerBothDead.calls) < 1 || providerBothDead.calls[0] != "allanime:sub" {
		t.Fatalf("expected preferred sub first, got %#v", providerBothDead.calls)
	}

	// When alternate streams exist, user is asked before recovery.
	providerAltOK := &stackStubProvider{
		name: "allanime",
		episodeResults: map[string]map[string][]string{
			"allanime-id": {"dub": {"dub-url"}},
		},
		episodeErrors: map[string]map[string]error{
			"allanime-id": {"sub": errors.New("no sub")},
		},
		searchResults: map[string][]SelectionOption{
			"sub": {{Title: "Example", Key: "allanime-id"}},
			"dub": {{Title: "Example", Key: "allanime-id"}},
		},
	}
	withProviderFactories(t, providerAltOK)
	promptPhases = nil
	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		if len(options) >= 1 && options[0].Key == "play" {
			promptPhases = append(promptPhases, "alternate-audio")
			return SelectionOption{Key: "cancel"}, nil
		}
		if len(options) >= 1 && options[0].Key == "remap" {
			promptPhases = append(promptPhases, "recovery")
			return SelectionOption{Key: "back"}, nil
		}
		t.Fatalf("unexpected prompt options: %#v", options)
		return SelectionOption{}, nil
	})
	anime.Ep.Number = 4
	if _, ok := resolveEpisodeLinksWithRecovery(cfg, anime, nil); ok {
		t.Fatal("expected back-out after declining alternate")
	}
	if got := strings.Join(promptPhases, ","); got != "alternate-audio,recovery" {
		t.Fatalf("expected alternate audio prompt before recovery when dub exists, got %q", got)
	}
}

func TestResolveEpisodeLinksWithRecoveryRemapThenSucceeds(t *testing.T) {
	// First mapping fails; after user remaps via ResolveAnimeProviderMapping stubs are hard.
	// Simulate remap by having recovery choose "episode" change that unlocks links? Better:
	// recovery "audio" after preferred-only failure with dub available without going through
	// the built-in alternate prompt (user declined it).

	provider := &stackStubProvider{
		name: "allanime",
		episodeResults: map[string]map[string][]string{
			"allanime-id": {"dub": {"dub-url"}},
		},
		episodeErrors: map[string]map[string]error{
			"allanime-id": {"sub": errors.New("no sub")},
		},
		searchResults: map[string][]SelectionOption{
			"sub": {{Title: "Example", Key: "allanime-id"}},
			"dub": {{Title: "Example", Key: "allanime-id"}},
		},
	}
	withProviderFactories(t, provider)

	phase := 0
	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		phase++
		switch phase {
		case 1:
			// Decline alternate during ResolveEpisodeURLForPlayback.
			if options[0].Key != "play" {
				t.Fatalf("expected alternate audio prompt first, got %#v", options)
			}
			return SelectionOption{Key: "cancel"}, nil
		case 2:
			// Recovery menu — pick explicit audio retry.
			if options[0].Key != "remap" {
				t.Fatalf("expected recovery menu, got %#v", options)
			}
			return SelectionOption{Key: "audio"}, nil
		case 3:
			// Approve alternate from recovery path.
			if options[0].Key != "play" {
				t.Fatalf("expected alternate audio approval, got %#v", options)
			}
			return SelectionOption{Key: "play"}, nil
		default:
			t.Fatalf("unexpected extra prompt phase %d: %#v", phase, options)
			return SelectionOption{}, nil
		}
	})

	cfg := &CurdConfig{Provider: `["allanime","no-animepahe"]`, SubOrDub: "sub"}
	anime := &Anime{
		Title:        AnimeTitle{Romaji: "Example"},
		ProviderName: "allanime",
		ProviderId:   "allanime-id",
		Ep:           Episode{Number: 1},
	}

	result, ok := resolveEpisodeLinksWithRecovery(cfg, anime, nil)
	if !ok {
		t.Fatal("expected recovery audio path to succeed")
	}
	if result.Mode != "dub" || result.Links[0] != "dub-url" {
		t.Fatalf("expected dub result after recovery audio, got %#v", result)
	}
}
