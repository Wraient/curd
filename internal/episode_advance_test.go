package internal

import "testing"

// resolveSlot maps a 0-based mpv playlist position to an episode using the
// frozen slot order built at startup. Titles are only a fallback for
// externally-modified queues (pos out of range) — never an override.
func TestResolveSlotPrefersFrozenOrderOverTitle(t *testing.T) {
	slots := []playlistSlot{
		{Episode: 3, Mode: "sub"},
		{Episode: 4, Mode: "sub"},
		{Episode: 5, Mode: "sub"},
		{Episode: 6, Mode: "sub"},
	}
	// mpv's own entry still shows the old title while the controller has
	// already moved on: the frozen slot (ep 5) must win, not the stale
	// title (ep 4). This is the reported 5-gets-4 defect in miniature.
	slot, source := resolveSlot(slots, 2, "Some Anime - Episode 4")
	if slot.Episode != 5 || source != slotOrder {
		t.Fatalf("expected slot ep 5 via order, got %+v via %v", slot, source)
	}
}

func TestResolveSlotFallsBackToTitleWhenPosInvalid(t *testing.T) {
	slots := []playlistSlot{
		{Episode: 5, Mode: "sub"},
	}
	slot, source := resolveSlot(slots, 99, "Some Anime - Episode 5")
	if slot.Episode != 5 || source != slotTitleFallback {
		t.Fatalf("expected ep 5 via title fallback, got %+v via %v", slot, source)
	}
}

func TestResolveSlotRejectsGarbage(t *testing.T) {
	if _, source := resolveSlot(nil, 0, "---"); source != slotInvalid {
		t.Fatalf("expected invalid for empty slots, got %v", source)
	}
	if _, source := resolveSlot([]playlistSlot{{Episode: 5}}, 7, "no numbers here"); source != slotInvalid {
		t.Fatalf("expected invalid for bad pos+title, got %v", source)
	}
}

func TestNextEpisodeNumberBasic(t *testing.T) {
	eps := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26}
	next, ok := nextEpisodeNumber(5, eps, 26, nil, false)
	if !ok || next != 6 {
		t.Fatalf("expected (6,true), got (%d,%v)", next, ok)
	}
}

func TestNextEpisodeNumberSeriesEnd(t *testing.T) {
	eps := []int{1, 2, 3}
	if next, ok := nextEpisodeNumber(3, eps, 3, nil, false); ok || next != 0 {
		t.Fatalf("expected done at series end, got (%d,%v)", next, ok)
	}
	// Unknown total still ends when the list is exhausted.
	if next, ok := nextEpisodeNumber(3, eps, 0, nil, false); ok || next != 0 {
		t.Fatalf("expected done when list exhausted, got (%d,%v)", next, ok)
	}
}

func TestNextEpisodeNumberSkipsMissing(t *testing.T) {
	eps := []int{1, 2, 3, 4, 5, 7, 8}
	next, ok := nextEpisodeNumber(5, eps, 8, nil, false)
	if !ok || next != 7 {
		t.Fatalf("expected (7,true) skipping missing ep 6, got (%d,%v)", next, ok)
	}
}

func TestNextEpisodeNumberFiller(t *testing.T) {
	eps := []int{5, 6, 7, 8}
	filler := []int{6, 7}
	if next, ok := nextEpisodeNumber(5, eps, 8, filler, true); !ok || next != 8 {
		t.Fatalf("expected (8,true) skipping fillers, got (%d,%v)", next, ok)
	}
	if next, ok := nextEpisodeNumber(5, eps, 8, filler, false); !ok || next != 6 {
		t.Fatalf("expected (6,true) keeping fillers, got (%d,%v)", next, ok)
	}
}

func TestNextEpisodeNumberStartAndSingle(t *testing.T) {
	if next, ok := nextEpisodeNumber(0, []int{1, 2, 3}, 3, nil, false); !ok || next != 1 {
		t.Fatalf("expected (1,true) for unwatched, got (%d,%v)", next, ok)
	}
	if next, ok := nextEpisodeNumber(1, []int{1}, 1, nil, false); ok || next != 0 {
		t.Fatalf("expected done for single-episode title, got (%d,%v)", next, ok)
	}
	if next, ok := nextEpisodeNumber(0, nil, 0, nil, false); ok || next != 0 {
		t.Fatalf("expected done for empty list, got (%d,%v)", next, ok)
	}
}

func TestNextEpisodeNumberBeyondList(t *testing.T) {
	eps := []int{1, 2, 3}
	if next, ok := nextEpisodeNumber(30, eps, 26, nil, false); ok || next != 0 {
		t.Fatalf("expected done past the list, got (%d,%v)", next, ok)
	}
}

func TestFilterSkippedEpisodes(t *testing.T) {
	eps := []int{1, 2, 3, 4, 5}
	filler := []int{2, 4}
	got := filterSkippedEpisodes(append([]int(nil), eps...), 1, filler, true)
	if len(got) != 3 || got[0] != 1 || got[1] != 3 || got[2] != 5 {
		t.Fatalf("expected [1 3 5], got %v", got)
	}
	// The episode actually playing is never removed, even if flagged filler.
	got = filterSkippedEpisodes(append([]int(nil), eps...), 2, filler, true)
	if len(got) != 4 || got[1] != 2 {
		t.Fatalf("expected current ep kept, got %v", got)
	}
	// Disabled skip keeps everything; empty filler list is a no-op.
	if got := filterSkippedEpisodes(append([]int(nil), eps...), 1, filler, false); len(got) != 5 {
		t.Fatalf("expected no filtering when disabled, got %v", got)
	}
	if got := filterSkippedEpisodes(append([]int(nil), eps...), 1, nil, true); len(got) != 5 {
		t.Fatalf("expected no filtering without filler data, got %v", got)
	}
}

// The controller owns advancement exactly while active: set claims it, a
// newer controller keeps it on clear, and a yield releases it for the TUI.
func TestPlaylistActiveLifecycle(t *testing.T) {
	a := &MPVPlaylistController{}
	b := &MPVPlaylistController{}
	setPlaylistControllerActive(a)
	if !MPVPlaylistActive() {
		t.Fatal("expected active after set")
	}
	clearPlaylistControllerActive(b)
	if !MPVPlaylistActive() {
		t.Fatal("newer controller must keep ownership on foreign clear")
	}
	a.yieldPlaylistToTUI("test yield")
	if MPVPlaylistActive() {
		t.Fatal("expected inactive after yield")
	}
	if !a.yielded.Load() {
		t.Fatal("expected yielded flag set")
	}
	clearPlaylistControllerActive(b)
	if MPVPlaylistActive() {
		t.Fatal("expected inactive after matching clear")
	}
}

// Transient unreadable picks must not yield (IPC blips recover), but a
// persistently broken queue must hand back to the TUI instead of gating it
// forever on a dead controller.
func TestUnresolvablePickYieldsAtThreshold(t *testing.T) {
	c := &MPVPlaylistController{}
	setPlaylistControllerActive(c)
	defer clearPlaylistControllerActive(c)
	for i := 0; i < unresolvableThreshold-1; i++ {
		c.noteUnresolvablePick("test")
		if c.yielded.Load() {
			t.Fatalf("yields too early at strike %d", i+1)
		}
		if !MPVPlaylistActive() {
			t.Fatal("must stay active before threshold")
		}
	}
	c.noteUnresolvablePick("test")
	if !c.yielded.Load() || MPVPlaylistActive() {
		t.Fatal("expected yield + inactive at threshold")
	}
}

// Series end drives the TUI handoff without touching mpv streams: after the
// last listed episode there is nothing to load, so the prompt's finish flow
// must take over exactly once.
func TestAdvancePastNaturalEndYieldsAtSeriesEnd(t *testing.T) {
	c := &MPVPlaylistController{
		anime:         &Anime{Ep: Episode{Number: 26}, TotalEpisodes: 26},
		preferredMode: "sub",
		currentMode:   "sub",
		episodeNums:   []int{25, 26},
	}
	c.currentPlaying = 26
	setPlaylistControllerActive(c)
	c.advancePastNaturalEnd()
	if !c.yielded.Load() {
		t.Fatal("expected yield at series end")
	}
	if MPVPlaylistActive() {
		t.Fatal("expected inactive after series-end yield")
	}
}
