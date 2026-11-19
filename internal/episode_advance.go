package internal

import (
	"sort"
)

// slotResolveSource records which evidence picked the episode for a
// playlist position. Frozen order always wins; the title is only a
// fallback for externally-modified queues and garbage is rejected.
type slotResolveSource int

const (
	slotInvalid slotResolveSource = iota
	slotOrder
	slotTitleFallback
)

// resolveSlot maps a 0-based mpv playlist position to an episode using the
// frozen slot order built at startup. Titles used to take precedence and a
// stale title after any queue reorder sent playback to the wrong episode
// (e.g. asking for ep 5 landing on ep 4); now a disagreeing title is
// ignored and only an out-of-range position falls back to title parsing.
func resolveSlot(slots []playlistSlot, pos int, title string) (playlistSlot, slotResolveSource) {
	if pos >= 0 && pos < len(slots) {
		return slots[pos], slotOrder
	}
	if ep, ok := episodeNumberFromSlotTitle(title, slots); ok {
		return ep, slotTitleFallback
	}
	return playlistSlot{}, slotInvalid
}

// episodeNumberFromSlotTitle parses a display title back to the matching
// slot entry. It only runs when the position is unusable, so it validates
// against the frozen list instead of trusting the title blindly.
func episodeNumberFromSlotTitle(title string, slots []playlistSlot) (playlistSlot, bool) {
	ep, _, ok := parsePlaylistEpisodeTitle(title, "")
	if !ok {
		return playlistSlot{}, false
	}
	for _, slot := range slots {
		if slot.Episode == ep {
			return slot, true
		}
	}
	return playlistSlot{}, false
}

// filterSkippedEpisodes removes filler episodes from a queue list when skip
// is set, but never the episode actually playing: the user explicitly
// started it, and late-arriving filler metadata must not hide the live
// entry. Runs in place on the same backing array; the caller must refresh
// any stored slice header (length changes).
func filterSkippedEpisodes(episodes []int, current int, filler []int, skip bool) []int {
	if !skip || len(filler) == 0 {
		return episodes
	}
	kept := episodes[:0]
	for _, ep := range episodes {
		if ep != current && IsEpisodeFiller(filler, ep) {
			continue
		}
		kept = append(kept, ep)
	}
	return kept
}

// nextEpisodeNumber decides which episode plays after current, given the
// provider's episode list. Episodes need not be contiguous: a missing
// number is skipped, never fabricated. It returns ok=false when there is
// nothing after current (series end, single-episode title watched, empty
// list, or current beyond the list).
//
//   - episodes is the provider's list (any order; sorted internally).
//   - totalEpisodes <= 0 means unknown (airing): the list decides.
//   - filler episodes are skipped only when skipFiller is set.
//   - current <= 0 (unwatched) yields the first episode.
func nextEpisodeNumber(current int, episodes []int, totalEpisodes int, filler []int, skipFiller bool) (int, bool) {
	ordered := append([]int(nil), episodes...)
	sort.Ints(ordered)
	// Deduplicate so a doubled provider entry cannot stall advancement.
	uniq := ordered[:0]
	for _, ep := range ordered {
		if len(uniq) == 0 || uniq[len(uniq)-1] != ep {
			uniq = append(uniq, ep)
		}
	}
	isFiller := func(ep int) bool {
		if !skipFiller {
			return false
		}
		for _, f := range filler {
			if f == ep {
				return true
			}
		}
		return false
	}
	for _, ep := range uniq {
		if ep <= 0 {
			continue
		}
		if ep > current && !isFiller(ep) {
			return ep, true
		}
	}
	return 0, false
}
