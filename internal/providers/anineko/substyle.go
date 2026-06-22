package anineko

import (
	"fmt"
	"strings"
	"sync"

	"github.com/wraient/curd/internal/curdhost"
)

var (
	subStyleChoiceMu sync.Mutex
	subStyleChosen   string
)

func normalizeSubStyle(style string) string {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "soft":
		return "soft"
	case "hard":
		return "hard"
	default:
		return "ask"
	}
}

func resolveSubStylePreference(fallback string) string {
	if curdhost.CurrentSubStyle != nil {
		if live := normalizeSubStyle(curdhost.CurrentSubStyle()); live != "ask" {
			return live
		}
	}

	subStyleChoiceMu.Lock()
	defer subStyleChoiceMu.Unlock()
	if subStyleChosen != "" {
		return subStyleChosen
	}
	return normalizeSubStyle(fallback)
}

func rememberSubStyleChoice(style string) {
	style = normalizeSubStyle(style)
	if style != "soft" && style != "hard" {
		return
	}

	subStyleChoiceMu.Lock()
	subStyleChosen = style
	subStyleChoiceMu.Unlock()

	if curdhost.PersistSubStylePreference != nil {
		_ = curdhost.PersistSubStylePreference(style)
	}
}

func chooseSubStyle(groups map[string][]string, preference string) (string, error) {
	preference = resolveSubStylePreference(preference)
	hasSoft := len(groups["sub"]) > 0
	hasHard := len(groups["hsub"]) > 0

	switch preference {
	case "soft":
		if hasSoft {
			return "soft", nil
		}
		if hasHard {
			return "hard", nil
		}
	case "hard":
		if hasHard {
			return "hard", nil
		}
		if hasSoft {
			return "soft", nil
		}
	case "ask":
		if hasSoft && hasHard {
			if curdhost.PromptSelect == nil {
				rememberSubStyleChoice("soft")
				return "soft", nil
			}
			if curdhost.Out != nil {
				curdhost.Out("Both soft-sub and hard-sub streams are available.")
			}
			selected, err := curdhost.PromptSelect([]curdhost.PromptOption{
				{Key: "soft", Label: "Soft sub (external subtitles)"},
				{Key: "hard", Label: "Hard sub (burned-in subtitles)"},
			})
			if err != nil {
				return "", err
			}
			chosen := "soft"
			if selected.Key == "hard" {
				chosen = "hard"
			}
			rememberSubStyleChoice(chosen)
			return chosen, nil
		}
		if hasSoft {
			return "soft", nil
		}
		if hasHard {
			return "hard", nil
		}
	}

	return "", fmt.Errorf("no sub streams found")
}

func embedURLsForSubStyle(groups map[string][]string, style string) []string {
	switch style {
	case "soft":
		return append([]string{}, groups["sub"]...)
	case "hard":
		return append([]string{}, groups["hsub"]...)
	default:
		return nil
	}
}

func embedURLsForMode(groups map[string][]string, mode, subStyle string) ([]string, error) {
	if strings.EqualFold(mode, "dub") {
		return groups["dub"], nil
	}

	chosen, err := chooseSubStyle(groups, subStyle)
	if err != nil {
		return nil, err
	}
	return embedURLsForSubStyle(groups, chosen), nil
}

func resetSubStyleForTest() {
	subStyleChoiceMu.Lock()
	subStyleChosen = ""
	subStyleChoiceMu.Unlock()
}
