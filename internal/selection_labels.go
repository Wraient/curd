package internal

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/wraient/curd/internal/providers/animepahe"
)

var selectionEpisodeCountRE = regexp.MustCompile(`\((\d+)\s+episodes?\)`)

func FormatAnimeSearchLabel(title string, episodes int) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	if episodes > 0 {
		return fmt.Sprintf("%s (%d episodes)", title, episodes)
	}
	return title
}

func TrackerEpisodeHint(config *CurdConfig, entry *Entry) string {
	if entry == nil || entry.Media.Episodes <= 0 {
		return ""
	}
	tracker := RemoteTrackingDisplayName(config)
	title := mediaDisplayTitle(entry.Media, config)
	if title == "" {
		return fmt.Sprintf("Your %s entry has %d episodes — pick the matching title below.", tracker, entry.Media.Episodes)
	}
	return fmt.Sprintf("Your %s entry for %q has %d episodes — pick the matching title below.", tracker, title, entry.Media.Episodes)
}

func episodeCountFromSelectionOption(option SelectionOption) (int, bool) {
	for _, source := range []string{option.Label, option.Title} {
		if source == "" {
			continue
		}
		if matches := selectionEpisodeCountRE.FindStringSubmatch(source); len(matches) > 1 {
			if count, err := strconv.Atoi(matches[1]); err == nil && count > 0 {
				return count, true
			}
		}
	}

	if option.ExtraData != nil {
		if item, ok := option.ExtraData.(animepahe.SearchItem); ok && item.Episodes > 0 {
			return item.Episodes, true
		}
	}

	return 0, false
}

func selectionOptionBaseTitle(option SelectionOption) string {
	if option.Title != "" {
		return strings.TrimSpace(option.Title)
	}

	label := strings.TrimSpace(option.Label)
	if label == "" {
		return ""
	}
	if idx := strings.Index(label, " ("); idx > 0 {
		return strings.TrimSpace(label[:idx])
	}
	if idx := strings.Index(label, " — "); idx > 0 {
		return strings.TrimSpace(label[:idx])
	}
	if idx := strings.Index(label, " ["); idx > 0 {
		return strings.TrimSpace(label[:idx])
	}
	return label
}

func ensureEpisodeCountInLabel(option SelectionOption) SelectionOption {
	if selectionEpisodeCountRE.MatchString(option.Label) {
		return option
	}

	count, ok := episodeCountFromSelectionOption(option)
	if !ok || count <= 0 {
		return option
	}

	baseTitle := selectionOptionBaseTitle(option)
	if baseTitle == "" {
		return option
	}

	providerSuffix := ""
	if idx := strings.LastIndex(option.Label, " ["); idx > 0 && strings.HasSuffix(option.Label, "]") {
		providerSuffix = option.Label[idx:]
	}

	option.Label = FormatAnimeSearchLabel(baseTitle, count) + providerSuffix
	if option.Title == "" {
		option.Title = baseTitle
	}
	return option
}

func ProviderSearchOptionsForDisplay(options []SelectionOption, entry *Entry) []SelectionOption {
	if len(options) == 0 {
		return options
	}

	targetEpisodes := 0
	if entry != nil {
		targetEpisodes = entry.Media.Episodes
	}

	display := make([]SelectionOption, len(options))
	for i, option := range options {
		display[i] = ensureEpisodeCountInLabel(option)
		if targetEpisodes <= 0 {
			continue
		}
		if count, ok := episodeCountFromSelectionOption(display[i]); ok && count == targetEpisodes {
			display[i].Label = display[i].Label + " ✓"
		}
	}
	return display
}

func ManualProviderSearchEnabled(config *CurdConfig) bool {
	return config != nil && config.ManualProviderSearch
}

func displayMediaFormat(format string) string {
	format = strings.TrimSpace(strings.ToUpper(format))
	switch format {
	case "TV":
		return "TV"
	case "TV_SHORT":
		return "TV Short"
	case "MOVIE":
		return "Movie"
	case "OVA":
		return "OVA"
	case "ONA":
		return "ONA"
	case "SPECIAL":
		return "Special"
	case "MUSIC":
		return "Music"
	default:
		return strings.ReplaceAll(format, "_", " ")
	}
}

func formatEpisodeCountHint(episodes int) string {
	if episodes > 0 {
		return fmt.Sprintf("%d episodes", episodes)
	}
	return "unknown episode count"
}

func ManualProviderSearchHint(config *CurdConfig, entry *Entry, query, subOrDub string) string {
	title := strings.TrimSpace(query)
	mediaType := ""
	episodes := 0

	if entry != nil {
		if trackerTitle := mediaDisplayTitle(entry.Media, config); trackerTitle != "" {
			title = trackerTitle
		}
		mediaType = displayMediaFormat(entry.Media.Format)
		episodes = entry.Media.Episodes
	}
	if title == "" {
		title = "unknown title"
	}

	mode := strings.ToLower(normalizeTranslationType(subOrDub))
	parts := []string{fmt.Sprintf("Searching for %q", title)}
	if mediaType != "" {
		parts = append(parts, mediaType)
	}
	parts = append(parts, formatEpisodeCountHint(episodes))
	if mode != "" {
		parts = append(parts, mode)
	}
	return strings.Join(parts, " · ") + " — select the matching provider entry below."
}

func promptProviderSearchSelectionWithHint(config *CurdConfig, options []SelectionOption, entry *Entry, hint string) (SelectionOption, error) {
	if strings.TrimSpace(hint) != "" {
		CurdOut(hint)
	}
	return DynamicSelect(ProviderSearchOptionsForDisplay(options, entry))
}

func promptProviderSearchSelection(config *CurdConfig, options []SelectionOption, entry *Entry) (SelectionOption, error) {
	return promptProviderSearchSelectionWithHint(config, options, entry, TrackerEpisodeHint(config, entry))
}
