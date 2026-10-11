package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Matches "Episode 12" / "Ep 12" in MPV playlist titles we set.
var playlistEpisodeTitleRE = regexp.MustCompile(`(?i)\b(?:episode|ep)\s*(\d+)\b`)

// Matches s=WWxHH inside our placeholder URLs (episode encoded in size).
var placeholderSizeRE = regexp.MustCompile(`s=(\d+)x(\d+)`)

const mpvPlaceholderDuration = 86400

// placeholderURL returns a playable dummy path unique for episode ep.
// Episode is encoded into lavfi size (fragments break lavfi in mpv).
func placeholderURL(ep int) string {
	if ep < 0 {
		ep = 0
	}
	// ep = (h-16)*200 + (w-16); w,h >= 16
	w := 16 + (ep % 200)
	h := 16 + (ep / 200)
	return fmt.Sprintf("av://lavfi:color=c=0x101010:s=%dx%d:d=%d", w, h, mpvPlaceholderDuration)
}

func parseEpisodeFromPlaceholder(path string) (int, bool) {
	m := placeholderSizeRE.FindStringSubmatch(path)
	if len(m) < 3 {
		return 0, false
	}
	w, err1 := strconv.Atoi(m[1])
	h, err2 := strconv.Atoi(m[2])
	if err1 != nil || err2 != nil || w < 16 || h < 16 {
		return 0, false
	}
	ep := (h-16)*200 + (w - 16)
	if ep <= 0 {
		return 0, false
	}
	return ep, true
}

const (
	mpvPlaylistIdleDelay     = 3 * time.Second
	mpvPlaylistPollInterval  = 400 * time.Millisecond
	mpvPlaylistStableSamples = 3
)

// mpvPlaylistSwitching is set while a playlist-driven episode switch is in
// progress. The main playback monitor must not treat transient time-pos loss
// (placeholder EOF / loadfile gap) as "user quit".
var mpvPlaylistSwitching atomic.Bool

// MPVPlaylistIsSwitching reports whether a playlist episode switch is underway.
func MPVPlaylistIsSwitching() bool {
	return mpvPlaylistSwitching.Load()
}

func beginMPVPlaylistSwitch() {
	mpvPlaylistSwitching.Store(true)
}

func endMPVPlaylistSwitch() {
	mpvPlaylistSwitching.Store(false)
}

// playlistSlot maps an MPV playlist index to curd episode + audio mode.
type playlistSlot struct {
	Episode int
	Mode    string // "sub" or "dub"
	Label   string
}

// MPVPlaylistController maintains a seamless MPV playlist of episodes (and
// optional alternate audio entry) without interrupting active playback while
// it is being built. Population happens only after playback is idle/stable.
//
// Episode numbers come from a single provider EpisodesList call (not per-episode
// stream probes). Placeholder rows use a temp M3U so MPV shows clean titles.
type MPVPlaylistController struct {
	config *CurdConfig
	anime  *Anime
	socket string
	done   <-chan struct{}

	mu             sync.Mutex
	slots          []playlistSlot
	suppress       atomic.Bool // ignore playlist-pos churn we cause ourselves
	lastPos        int
	built          bool
	preferredMode  string
	hasAlternate   bool
	alternateMode  string
	currentPlaying int // episode number currently intended
	currentMode    string
	episodeNums    []int // provider episode list (sorted), preferred mode
}

// StartMPVPlaylistController waits until playback is stable, then builds the
// episode playlist and watches for user selections. Safe to call in a goroutine.
// Does nothing for android-intent / empty sockets or when disabled in config.
func StartMPVPlaylistController(config *CurdConfig, anime *Anime, socket string, done <-chan struct{}) {
	if config == nil || anime == nil || !config.MpvEpisodePlaylist {
		return
	}
	if socket == "" || socket == "android-intent" {
		return
	}

	c := &MPVPlaylistController{
		config:         config,
		anime:          anime,
		socket:         socket,
		done:           done,
		lastPos:        -1,
		preferredMode:  normalizeTranslationType(config.SubOrDub),
		currentPlaying: anime.Ep.Number,
		currentMode:    normalizeTranslationType(config.SubOrDub),
	}
	if c.preferredMode == "" {
		c.preferredMode = "sub"
	}
	c.currentMode = c.preferredMode
	c.alternateMode = alternateTranslationType(c.preferredMode)

	go c.run()
}

func (c *MPVPlaylistController) closed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c *MPVPlaylistController) run() {
	if !c.waitUntilIdleStable() {
		return
	}
	if c.closed() {
		return
	}

	// Build episode rows without touching the currently playing demuxer path
	// beyond append/insert of titled placeholders (one EpisodesList fetch).
	if err := c.buildEpisodePlaylist(); err != nil {
		Log(fmt.Sprintf("MPV playlist build failed: %v", err))
		// Still watch in case a partial list exists.
	} else {
		c.built = true
		Log(fmt.Sprintf("MPV episode playlist ready (%d episodes)", len(c.episodeNums)))
	}

	if c.closed() {
		return
	}

	// Probe alternate audio only for the *current* episode — never blocks first frame.
	c.probeAndAttachAlternateAudio()

	c.watchPlaylistSelection()
}

// waitUntilIdleStable waits until MPV reports a real time-pos a few times in a
// row so we do not mutate the playlist during initial buffering.
func (c *MPVPlaylistController) waitUntilIdleStable() bool {
	deadline := time.Now().Add(45 * time.Second)
	stable := 0
	// Initial grace so loadfile/subs settle.
	timer := time.NewTimer(mpvPlaylistIdleDelay)
	defer timer.Stop()
	select {
	case <-c.done:
		return false
	case <-timer.C:
	}

	for time.Now().Before(deadline) {
		if c.closed() {
			return false
		}
		pos, err := MPVSendCommand(c.socket, []interface{}{"get_property", "time-pos"})
		if err == nil && pos != nil {
			if mpvNumber(pos) >= 0 {
				stable++
				if stable >= mpvPlaylistStableSamples {
					return true
				}
				time.Sleep(mpvPlaylistPollInterval)
				continue
			}
		}
		stable = 0
		time.Sleep(mpvPlaylistPollInterval)
	}
	Log("MPV playlist: playback never became stable; skipping playlist build")
	return false
}

// fetchEpisodeNumbers loads the full episode number list from the provider in one
// shot via EpisodesList. Never resolves per-episode streams (avoids 429s).
func (c *MPVPlaylistController) fetchEpisodeNumbers() []int {
	providerName, providerID := AnimeProviderID(c.anime)
	if providerID != "" {
		qualified := QualifyProviderID(providerName, providerID)
		// Preferred mode only — one list request. Do not also hit alternate mode
		// (GetProviderTotalEpisodes does that; we deliberately do not).
		list, err := EpisodesList(qualified, c.preferredMode)
		if err != nil {
			Log(fmt.Sprintf("MPV playlist: EpisodesList(%s) failed: %v — falling back", providerName, err))
		} else if nums := parseEpisodeNumberList(list); len(nums) > 0 {
			return nums
		}
	}

	// Fallback: contiguous 1..N from AniList / known total (still no stream probes).
	total := c.anime.TotalEpisodes
	if total <= 0 && providerID != "" {
		if n, err := GetProviderTotalEpisodes(QualifyProviderID(providerName, providerID), c.preferredMode); err == nil {
			total = n
			c.anime.TotalEpisodes = n
		}
	}
	if total <= 0 {
		return nil
	}
	nums := make([]int, 0, total)
	for ep := 1; ep <= total; ep++ {
		nums = append(nums, ep)
	}
	return nums
}

func parseEpisodeNumberList(list []string) []int {
	seen := make(map[int]struct{}, len(list))
	for _, raw := range list {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		// Accept "12" or "12.0"
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil || f <= 0 {
			continue
		}
		ep := int(f)
		if float64(ep) != f {
			// Fractional episode markers (specials) — keep as floor if whole part > 0
			if ep <= 0 {
				continue
			}
		}
		seen[ep] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	nums := make([]int, 0, len(seen))
	for ep := range seen {
		nums = append(nums, ep)
	}
	sort.Ints(nums)
	return nums
}

func (c *MPVPlaylistController) episodeLabel(ep int, mode string) string {
	name := strings.TrimSpace(GetAnimeName(*c.anime))
	base := fmt.Sprintf("Episode %d", ep)
	if name != "" {
		base = fmt.Sprintf("%s - Episode %d", name, ep)
	}
	if c.anime.FillerEpisodes != nil && IsEpisodeFiller(c.anime.FillerEpisodes, ep) {
		base += " [Filler]"
	}
	mode = normalizeTranslationType(mode)
	// Tag the row when its translation mode differs from what is currently
	// playing, so the alternate track is always obvious (in dub, a plain sub row
	// still shows "(SUB)"; in sub, the dub row shows "(DUB)").
	if c.currentMode != "" && mode != c.currentMode {
		base += " (" + strings.ToUpper(mode) + ")"
	}
	if ep == c.currentPlaying && mode == c.currentMode {
		base += "  ◀"
	}
	return base
}

func (c *MPVPlaylistController) buildEpisodePlaylist() error {
	episodes := c.fetchEpisodeNumbers()
	if len(episodes) == 0 {
		return fmt.Errorf("no episode list from provider")
	}
	c.episodeNums = episodes
	if max := episodes[len(episodes)-1]; max > c.anime.TotalEpisodes {
		c.anime.TotalEpisodes = max
	}

	currentEp := c.anime.Ep.Number
	if currentEp < 1 {
		currentEp = episodes[0]
	}
	// Ensure current episode is in the list (AniList progress may point at an ep
	// the provider still lists under a different numbering — keep a slot).
	if !containsInt(episodes, currentEp) {
		episodes = append(episodes, currentEp)
		sort.Ints(episodes)
		c.episodeNums = episodes
	}

	c.suppress.Store(true)
	defer c.suppress.Store(false)

	// Prefer title display in the playlist OSD when mpv supports it.
	_, _ = MPVSendCommand(c.socket, []interface{}{"set_property", "osd-playlist-entry", "title"})

	// Title the currently playing item (force-media-title sticks for the active entry).
	curLabel := c.episodeLabel(currentEp, c.currentMode)
	_, _ = MPVSendCommand(c.socket, []interface{}{"set_property", "force-media-title", curLabel})
	_, _ = MPVSendCommand(c.socket, []interface{}{"set_property", "title", curLabel})

	var before, after []playlistSlot
	for _, ep := range episodes {
		if ep < currentEp {
			before = append(before, playlistSlot{
				Episode: ep,
				Mode:    c.preferredMode,
				Label:   c.episodeLabel(ep, c.preferredMode),
			})
		} else if ep > currentEp {
			after = append(after, playlistSlot{
				Episode: ep,
				Mode:    c.preferredMode,
				Label:   c.episodeLabel(ep, c.preferredMode),
			})
		}
	}

	// Future episodes: one M3U loadlist append (titled placeholders, no stream probes).
	if len(after) > 0 {
		if err := c.loadTitledPlaceholders(after, false); err != nil {
			Log(fmt.Sprintf("playlist append future: %v", err))
		}
	}

	// Previous episodes: append via M3U then playlist-move to the front.
	if len(before) > 0 {
		if err := c.loadTitledPlaceholders(before, true); err != nil {
			Log(fmt.Sprintf("playlist insert previous: %v", err))
		}
	}

	// Rebuild slot map: before + current + after (preferred mode).
	slots := make([]playlistSlot, 0, len(episodes))
	slots = append(slots, before...)
	slots = append(slots, playlistSlot{
		Episode: currentEp,
		Mode:    c.preferredMode,
		Label:   curLabel,
	})
	slots = append(slots, after...)

	c.mu.Lock()
	c.slots = slots
	c.currentPlaying = currentEp
	c.mu.Unlock()

	wantPos := len(before)
	c.lastPos = wantPos
	if pos, err := c.playlistPos(); err == nil {
		c.lastPos = pos
		if pos != wantPos {
			Log(fmt.Sprintf("MPV playlist-pos=%d want=%d (leaving as-is to avoid stutter)", pos, wantPos))
		}
	}
	return nil
}

// loadTitledPlaceholders writes a temp M3U with #EXTINF titles and loadlists it.
// If moveToFront is true, entries are moved to the start of the playlist (preserving order).
func (c *MPVPlaylistController) loadTitledPlaceholders(slots []playlistSlot, moveToFront bool) error {
	if len(slots) == 0 {
		return nil
	}
	path, err := c.writePlaceholderM3U(slots)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(path) }()

	beforeCount, _ := c.playlistCount()

	if _, err := MPVSendCommand(c.socket, []interface{}{"loadlist", path, "append"}); err != nil {
		return fmt.Errorf("loadlist: %w", err)
	}

	afterCount, err := c.playlistCount()
	if err != nil {
		return err
	}
	added := afterCount - beforeCount
	if added <= 0 {
		return fmt.Errorf("loadlist added 0 entries")
	}

	if !moveToFront {
		return nil
	}

	// Move the newly appended block to the front, preserving relative order.
	// Start: [current, ..., NEW0, NEW1, ...] → [NEW0, NEW1, ..., current, ...]
	for i := 0; i < added; i++ {
		from := beforeCount + i // shifts as we insert earlier
		to := i
		if _, err := MPVSendCommand(c.socket, []interface{}{"playlist-move", from, to}); err != nil {
			Log(fmt.Sprintf("playlist-move %d→%d: %v", from, to, err))
		}
	}
	return nil
}

func (c *MPVPlaylistController) writePlaceholderM3U(slots []playlistSlot) (string, error) {
	dir := os.TempDir()
	if c.config != nil && strings.TrimSpace(c.config.StoragePath) != "" {
		dir = os.ExpandEnv(c.config.StoragePath)
		_ = os.MkdirAll(dir, 0o755)
	}
	f, err := os.CreateTemp(dir, "curd-playlist-*.m3u")
	if err != nil {
		return "", err
	}
	path := f.Name()

	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for _, s := range slots {
		title := sanitizeM3UTitle(s.Label)
		b.WriteString("#EXTINF:-1,")
		b.WriteString(title)
		b.WriteByte('\n')
		// Unique URL per episode so mpv actually changes playlist-pos on select.
		b.WriteString(placeholderURL(s.Episode))
		b.WriteByte('\n')
	}
	if _, err := f.WriteString(b.String()); err != nil {
		f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// sanitizeM3UTitle keeps #EXTINF titles single-line and free of control chars.
func sanitizeM3UTitle(title string) string {
	title = strings.ReplaceAll(title, "\r", " ")
	title = strings.ReplaceAll(title, "\n", " ")
	title = strings.TrimSpace(title)
	if title == "" {
		return "Episode"
	}
	return title
}

func containsInt(nums []int, want int) bool {
	for _, n := range nums {
		if n == want {
			return true
		}
	}
	return false
}

func (c *MPVPlaylistController) playlistPos() (int, error) {
	v, err := MPVSendCommand(c.socket, []interface{}{"get_property", "playlist-pos"})
	if err != nil {
		return -1, err
	}
	return int(mpvNumber(v) + 0.5), nil
}

func (c *MPVPlaylistController) playlistCount() (int, error) {
	v, err := MPVSendCommand(c.socket, []interface{}{"get_property", "playlist-count"})
	if err != nil {
		return 0, err
	}
	return int(mpvNumber(v) + 0.5), nil
}

// probeAndAttachAlternateAudio checks whether the opposite sub/dub mode has a
// stream for the *current* episode only (one resolve — not every episode).
func (c *MPVPlaylistController) probeAndAttachAlternateAudio() {
	if c.closed() || c.anime == nil {
		return
	}
	alt := c.alternateMode
	cfg := *c.config
	cfg.SubOrDub = alt

	// Background resolve — does not touch MPV until we know a stream exists.
	result, err := ResolveEpisodeURL(cfg, c.anime, c.currentPlaying)
	if err != nil || len(result.Links) == 0 {
		Log(fmt.Sprintf("MPV playlist: no %s stream for ep %d (%v)", alt, c.currentPlaying, err))
		c.hasAlternate = false
		return
	}
	c.hasAlternate = true

	c.suppress.Store(true)
	defer c.suppress.Store(false)

	c.mu.Lock()
	curIdx := c.episodeIndexLocked(c.currentPlaying, c.currentMode)
	if curIdx < 0 {
		curIdx = 0
	}
	label := c.episodeLabel(c.currentPlaying, alt)
	insertAt := curIdx + 1
	c.mu.Unlock()

	slot := playlistSlot{Episode: c.currentPlaying, Mode: alt, Label: label}
	if err := c.insertTitledPlaceholderAt(insertAt, slot); err != nil {
		Log(fmt.Sprintf("MPV playlist: failed to add %s entry: %v", alt, err))
		return
	}

	c.mu.Lock()
	if insertAt >= len(c.slots) {
		c.slots = append(c.slots, slot)
	} else {
		c.slots = append(c.slots[:insertAt], append([]playlistSlot{slot}, c.slots[insertAt:]...)...)
	}
	c.mu.Unlock()
	Log(fmt.Sprintf("MPV playlist: added %s option for ep %d", alt, c.currentPlaying))
}

// insertTitledPlaceholderAt appends a single titled M3U entry then moves it to index.
func (c *MPVPlaylistController) insertTitledPlaceholderAt(index int, slot playlistSlot) error {
	countBefore, _ := c.playlistCount()
	if err := c.loadTitledPlaceholders([]playlistSlot{slot}, false); err != nil {
		return err
	}
	countAfter, err := c.playlistCount()
	if err != nil {
		return err
	}
	if countAfter <= countBefore {
		return fmt.Errorf("placeholder not added")
	}
	from := countAfter - 1
	if from == index {
		return nil
	}
	if index < 0 {
		index = 0
	}
	if index > from {
		index = from
	}
	_, err = MPVSendCommand(c.socket, []interface{}{"playlist-move", from, index})
	return err
}

func (c *MPVPlaylistController) watchPlaylistSelection() {
	// Detect selection primarily via path: when the user picks another playlist
	// row, mpv loads our unique lavfi placeholder. Polling playlist-pos alone
	// missed jumps (logs showed time-pos drop with no "pos changed").
	Log("MPV playlist: watching path + playlist-pos for episode picks")
	poll := 150 * time.Millisecond
	// Sample tracking independent of the (possibly stale) cached lastPos so we
	// never silently miss an mpv path/pos transition.
	var samplePath string
	var samplePos int
	haveSample := false
	lastHeartbeat := time.Time{}

	for {
		if c.closed() {
			return
		}
		if !IsMPVRunning(c.socket) {
			// A transient IPC blip (mpv can briefly refuse the unix socket while
			// alive) must NOT kill the only episode-selection watcher. Retry on
			// the next tick; true termination is signalled via the done channel.
			time.Sleep(poll)
			continue
		}
		if c.suppress.Load() || MPVPlaylistIsSwitching() {
			time.Sleep(poll)
			continue
		}

		path := mpvStringProperty(c.socket, "path")
		pos, posErr := c.playlistPos()

		// Heartbeat ~once a second: captures mpv's real reported state over time
		// even when nothing changes, so we can prove what mpv does on a click.
		if time.Since(lastHeartbeat) >= time.Second {
			lastHeartbeat = time.Now()
			count, _ := c.playlistCount()
			Log(fmt.Sprintf("MPV playlist: HB pos=%d count=%d path=%s", pos, count, truncateForLog(path, 45)))
		}

		if !haveSample || path != samplePath || (posErr == nil && pos != samplePos) {
			samplePath = path
			samplePos = pos
			haveSample = true
			if isPlaceholderPath(path) {
				if ep, ok := parseEpisodeFromPlaceholder(path); ok {
					Log(fmt.Sprintf("MPV playlist: SAMPLE placeholder ep=%d path=%s", ep, truncateForLog(path, 60)))
				} else {
					Log(fmt.Sprintf("MPV playlist: SAMPLE placeholder path=%s", truncateForLog(path, 60)))
				}
			} else if posErr == nil {
				Log(fmt.Sprintf("MPV playlist: SAMPLE pos=%d path=%s", pos, truncateForLog(path, 50)))
			}
		}

		// --- Primary: path became a placeholder (a row was picked) ---
		// Resolve the episode from the frozen index + row title ONLY. The lavfi
		// filename's WxH encoding is unreliable (decodes to random episodes like
		// 192/300) and must never drive which episode we load.
		if isPlaceholderPath(path) {
			// Debounce: confirm the placeholder is still the active path (scrub).
			time.Sleep(80 * time.Millisecond)
			if !isPlaceholderPath(mpvStringProperty(c.socket, "path")) {
				continue
			}

			targetPos := pos
			if posErr != nil || pos < 0 {
				targetPos, _ = c.playlistPos()
			}
			slot, slotErr := c.resolveSlotAt(targetPos)
			if slotErr != nil {
				Log(fmt.Sprintf("MPV playlist: placeholder pick unresolvable at pos %d: %v", targetPos, slotErr))
				c.lastPos = targetPos
				continue
			}

			c.mu.Lock()
			curEp := c.currentPlaying
			c.mu.Unlock()

			Log(fmt.Sprintf("MPV playlist: DETECT placeholder row pos=%d → ep %d (%s) (was ep=%d)",
				targetPos, slot.Episode, slot.Mode, curEp))

			beginMPVPlaylistSwitch()
			_, _ = MPVSendCommand(c.socket, []interface{}{"set_property", "pause", true})
			_, _ = MPVSendCommand(c.socket, []interface{}{"show-text", fmt.Sprintf("Loading episode %d…", slot.Episode), 5000})

			c.lastPos = targetPos
			c.handlePlaylistJumpSlot(slot, curEp)
			continue
		}

		// --- Secondary: playlist-pos moved ---
		// IMPORTANT: freeze the *first* new index and resolve that entry immediately.
		// Re-reading pos after a sleep was overwriting 38→39 (current) so we always
		// reloaded ep 40 when the user clicked 39 (see debug: DETECT 39→38 then
		// "pos 39 title Episode 40").
		if posErr == nil && pos >= 0 && pos != c.lastPos {
			targetPos := pos
			fromPos := c.lastPos
			Log(fmt.Sprintf("MPV playlist: DETECT pos %d → %d (path=%s)", fromPos, targetPos, truncateForLog(path, 50)))

			// Resolve the entry the user landed on *now*, before mpv can snap back.
			slot, slotErr := c.resolveSlotAt(targetPos)
			if slotErr != nil {
				Log(fmt.Sprintf("MPV playlist: resolve pos %d failed: %v", targetPos, slotErr))
				// Still try path / slots fallback inside handlePlaylistJump.
				beginMPVPlaylistSwitch()
				_, _ = MPVSendCommand(c.socket, []interface{}{"set_property", "pause", true})
				c.lastPos = targetPos
				c.handlePlaylistJump(targetPos)
					continue
			}

			c.mu.Lock()
			curEp := c.currentPlaying
			c.mu.Unlock()

			Log(fmt.Sprintf("MPV playlist: frozen target pos=%d → ep %d (cur ep %d)", targetPos, slot.Episode, curEp))

			// If resolve says same episode as current and path is still the live stream,
			// this was a false pos blip — ignore.
			if slot.Episode == curEp && !isPlaceholderPath(path) {
				Log(fmt.Sprintf("MPV playlist: ignore pos blip %d→%d (still ep %d, live path)", fromPos, targetPos, curEp))
				c.lastPos = targetPos
				// If pos really moved within same ep (dub row), still switch mode.
				if normalizeTranslationType(slot.Mode) != normalizeTranslationType(c.currentMode) {
					beginMPVPlaylistSwitch()
					_, _ = MPVSendCommand(c.socket, []interface{}{"set_property", "pause", true})
					c.handlePlaylistJumpSlot(slot, curEp)
						}
				time.Sleep(poll)
				continue
			}

			beginMPVPlaylistSwitch()
			_, _ = MPVSendCommand(c.socket, []interface{}{"set_property", "pause", true})
			_, _ = MPVSendCommand(c.socket, []interface{}{"show-text", fmt.Sprintf("Loading episode %d…", slot.Episode), 5000})
			c.lastPos = targetPos
			c.handlePlaylistJumpSlot(slot, curEp)
			continue
		}

		if path != "" {

		}
		time.Sleep(poll)
	}
}

func isPlaceholderPath(path string) bool {
	return strings.HasPrefix(path, "av://lavfi:") || strings.Contains(path, "lavfi:color=")
}

// handlePlaylistJumpSlot switches to a known target slot (from path or pos detection).
func (c *MPVPlaylistController) handlePlaylistJumpSlot(slot playlistSlot, curEp int) {
	curMode := c.currentMode
	if curMode == "" {
		curMode = c.preferredMode
	}
	beginMPVPlaylistSwitch()
	_, _ = MPVSendCommand(c.socket, []interface{}{"set_property", "pause", true})

	leaveAction := playlistLeaveNone
	if slot.Episode != curEp {
		pct := PercentageWatched(c.anime.Ep.Player.PlaybackTime, c.anime.Ep.Duration)
		threshold := 85
		if c.config != nil && c.config.PercentageToMarkComplete > 0 {
			threshold = c.config.PercentageToMarkComplete
		}
		// Never interrupt playback with a terminal menu while the user is in
		// (possibly fullscreen) MPV. Resolve the remote action silently: backward
		// jumps never regress upstream progress, +1 follows the standard "next"
		// auto-mark behavior, other forward skips leave progress untouched.
		leaveAction = resolvePlaylistLeaveDefault(curEp, slot.Episode, pct, threshold)
		Log(fmt.Sprintf("MPV playlist: SWITCH %d → %d (%s) leave=%s (silent, no session menu)",
			curEp, slot.Episode, slot.Mode, leaveAction))
	} else {
		leaveAction = playlistLeaveNone
		Log(fmt.Sprintf("MPV playlist: reloading real stream for ep %d (was on placeholder)", curEp))
	}

	_, _ = MPVSendCommand(c.socket, []interface{}{"show-text", fmt.Sprintf("Loading episode %d…", slot.Episode), 8000})
	Log(fmt.Sprintf("MPV playlist: SWITCH ep %d → %d (%s) leave=%s", curEp, slot.Episode, slot.Mode, leaveAction))
	CurdOut(fmt.Sprintf("Loading episode %d…", slot.Episode))

	leftEp := curEp
	if err := c.playSlot(slot); err != nil {
		Log(fmt.Sprintf("MPV playlist: failed to play ep %d: %v", slot.Episode, err))
		CurdOut(fmt.Sprintf("Could not play episode %d: %v", slot.Episode, err))
		if err2 := c.playSlot(playlistSlot{Episode: curEp, Mode: curMode, Label: c.episodeLabel(curEp, curMode)}); err2 != nil {
			Log(fmt.Sprintf("MPV playlist: recovery play failed: %v", err2))
			endMPVPlaylistSwitch()
		}
		return
	}

	c.finalizePlaylistEpisodeChange(leftEp, slot.Episode, slot.Mode, leaveAction)

	go func() {
		time.Sleep(mpvPlaylistIdleDelay)
		if c.closed() {
			return
		}
		if err := c.rebuildPlaylistAroundCurrent(); err != nil {
			Log(fmt.Sprintf("MPV playlist rebuild after switch: %v", err))
		}
	}()
}

// playlistLeaveAction is the user's choice when leaving an episode via playlist.
type playlistLeaveAction string

const (
	playlistLeaveNone     playlistLeaveAction = "none"     // play target; last-played only (no remote)
	playlistLeaveMarkLeft playlistLeaveAction = "mark"     // mark left episode watched remotely
	playlistLeaveSetWatch playlistLeaveAction = "setwatch" // set remote progress to toEp-1
	playlistLeaveCancel   playlistLeaveAction = "cancel"   // stay on current episode
)

// promptPlaylistEpisodeLeave asks how to handle progress when changing episodes.
// Overridable in tests. +1 (next) is auto; other jumps prompt.
var promptPlaylistEpisodeLeave = defaultPromptPlaylistEpisodeLeave

func defaultPromptPlaylistEpisodeLeave(fromEp, toEp int, percentageWatched float64, threshold int) (playlistLeaveAction, error) {
	// Same episode (mode toggle only) — nothing to ask.
	if toEp == fromEp {
		return playlistLeaveNone, nil
	}

	nearlyDone := int(percentageWatched) >= threshold && percentageWatched > 0
	pctLabel := fmt.Sprintf("%.0f%%", percentageWatched)
	if percentageWatched <= 0 {
		pctLabel = "just started"
	}

	// Sequential next episode (+1): no menu — same spirit as normal "next ep".
	if toEp == fromEp+1 {
		if nearlyDone {
			CurdOut(fmt.Sprintf("Next episode %d · marking %d watched (was %s)", toEp, fromEp, pctLabel))
			return playlistLeaveMarkLeft, nil
		}
		CurdOut(fmt.Sprintf("Next episode %d · last played updated", toEp))
		return playlistLeaveNone, nil
	}

	// Non-linear jump — ask what to do with upstream progress.
	CurdOut(fmt.Sprintf("Jump: episode %d (%s) → episode %d", fromEp, pctLabel, toEp))

	options := []SelectionOption{
		{Key: "none", Label: fmt.Sprintf("▶ Play episode %d only (don’t change AniList/MAL)", toEp)},
		{Key: "mark", Label: fmt.Sprintf("✓ Mark episode %d watched, then play %d", fromEp, toEp)},
		{Key: "setwatch", Label: fmt.Sprintf("📌 Set progress to %d (watching %d), play %d", maxInt(0, toEp-1), toEp, toEp)},
		{Key: "cancel", Label: fmt.Sprintf("↩ Cancel — stay on episode %d", fromEp)},
	}
	// If they were almost done, put "mark left" first.
	if nearlyDone {
		options = []SelectionOption{
			{Key: "mark", Label: fmt.Sprintf("✓ Mark episode %d watched (you were %s), play %d", fromEp, pctLabel, toEp)},
			{Key: "none", Label: fmt.Sprintf("▶ Play episode %d only (don’t change AniList/MAL)", toEp)},
			{Key: "setwatch", Label: fmt.Sprintf("📌 Set progress to %d (watching %d), play %d", maxInt(0, toEp-1), toEp, toEp)},
			{Key: "cancel", Label: fmt.Sprintf("↩ Cancel — stay on episode %d", fromEp)},
		}
	}

	selected, err := promptSelectOrdered(options)
	if err != nil {
		// User already picked a row in MPV — never cancel back to the old episode.
		Log(fmt.Sprintf("playlist jump prompt failed (%v); playing %d without remote update", err, toEp))
		return playlistLeaveNone, nil
	}
	if selected.Key == "cancel" || SelectionMeansQuit(selected) || SelectionMeansBack(selected) {
		return playlistLeaveCancel, nil
	}
	switch selected.Key {
	case "mark":
		return playlistLeaveMarkLeft, nil
	case "setwatch":
		return playlistLeaveSetWatch, nil
	case "none", "switch":
		return playlistLeaveNone, nil
	default:
		return playlistLeaveNone, nil
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// resolvePlaylistLeaveDefault decides what to do with upstream progress after an
// MPV-playlist episode pick WITHOUT prompting mid-playback. Rules:
//   - same episode                       → nothing (mode toggle / reload)
//   - going backward (toEp < fromEp)     → nothing; never regress the tracker
//   - forward next (+1) nearly finished  → mark the left episode watched
//   - any other forward jump             → nothing (surfaced at session end)
func resolvePlaylistLeaveDefault(fromEp, toEp int, percentageWatched float64, threshold int) playlistLeaveAction {
	if toEp <= 0 || toEp == fromEp {
		return playlistLeaveNone
	}
	if toEp < fromEp {
		return playlistLeaveNone
	}
	if toEp == fromEp+1 {
		nearlyDone := int(percentageWatched) >= threshold && percentageWatched > 0
		if nearlyDone {
			return playlistLeaveMarkLeft
		}
	}
	return playlistLeaveNone
}

// resolveSlotAt figures out which episode the user selected at playlist index pos.
// Order: placeholder filename encode → title → our slots map.
// Never re-query playlist-pos here — caller freezes the index.
func (c *MPVPlaylistController) resolveSlotAt(pos int) (playlistSlot, error) {
	filename, title := c.playlistEntryMeta(pos)
	Log(fmt.Sprintf("MPV playlist: resolve pos=%d filename=%q title=%q", pos, truncateForLog(filename, 60), title))

	if title != "" {
		if ep, mode, ok := parsePlaylistEpisodeTitle(title, c.preferredMode); ok {
			Log(fmt.Sprintf("MPV playlist: pos %d title %q → ep %d (%s)", pos, title, ep, mode))
			return playlistSlot{Episode: ep, Mode: mode, Label: title}, nil
		}
		Log(fmt.Sprintf("MPV playlist: pos %d title %q (unparsed) file=%q", pos, title, truncateForLog(filename, 40)))
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if pos < 0 || pos >= len(c.slots) {
		return playlistSlot{}, fmt.Errorf("playlist pos %d out of range (slots=%d)", pos, len(c.slots))
	}
	slot := c.slots[pos]
	Log(fmt.Sprintf("MPV playlist: pos %d slots fallback → ep %d", pos, slot.Episode))
	return slot, nil
}

func (c *MPVPlaylistController) playlistEntryMeta(pos int) (filename, title string) {
	v, err := MPVSendCommand(c.socket, []interface{}{"get_property", "playlist"})
	if err != nil || v == nil {
		return "", ""
	}
	entries, ok := v.([]interface{})
	if !ok || pos < 0 || pos >= len(entries) {
		return "", ""
	}
	entry, ok := entries[pos].(map[string]interface{})
	if !ok {
		return "", ""
	}
	if f, ok := entry["filename"].(string); ok {
		filename = f
	}
	if t, ok := entry["title"].(string); ok {
		title = strings.TrimSpace(t)
	}
	return filename, title
}

func parsePlaylistEpisodeTitle(title, preferredMode string) (ep int, mode string, ok bool) {
	mode = normalizeTranslationType(preferredMode)
	if mode == "" {
		mode = "sub"
	}
	upper := strings.ToUpper(title)
	if strings.Contains(upper, "(DUB)") || strings.Contains(upper, "· DUB") || strings.HasSuffix(strings.TrimSpace(upper), "DUB") {
		mode = "dub"
	} else if strings.Contains(upper, "(SUB)") {
		mode = "sub"
	}
	m := playlistEpisodeTitleRE.FindStringSubmatch(title)
	if len(m) < 2 {
		return 0, mode, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0, mode, false
	}
	return n, mode, true
}

func (c *MPVPlaylistController) handlePlaylistJump(pos int) {
	curEp := c.currentPlaying

	slot, err := c.resolveSlotAt(pos)
	if err != nil {
		Log(fmt.Sprintf("MPV playlist: resolve slot at %d: %v", pos, err))
		// Path may still be a placeholder — try path-based parse of active media.
		if ep, ok := parseEpisodeFromPlaceholder(mpvStringProperty(c.socket, "path")); ok {
			slot = playlistSlot{Episode: ep, Mode: c.preferredMode, Label: c.episodeLabel(ep, c.preferredMode)}
			c.handlePlaylistJumpSlot(slot, curEp)
			return
		}
		_, _ = MPVSendCommand(c.socket, []interface{}{"set_property", "pause", false})
		endMPVPlaylistSwitch()
		return
	}

	c.handlePlaylistJumpSlot(slot, curEp)
}

// finalizePlaylistEpisodeChange updates local history, last-played id, episode
// metadata, optional remote progress, and prefetches the next episode.
func (c *MPVPlaylistController) finalizePlaylistEpisodeChange(fromEp, toEp int, mode string, leave playlistLeaveAction) {
	if c.anime == nil || c.config == nil {
		return
	}
	anime := c.anime
	mode = normalizeTranslationType(mode)
	if mode == "" {
		mode = c.preferredMode
	}

	// Keep config + controller mode in sync (sub/dub playlist entries).
	if mode != "" {
		c.config.SubOrDub = mode
		c.currentMode = mode
	}

	// Clear stale next-episode prefetch from the previous number.
	anime.Ep.NextEpisode = NextEpisode{}
	anime.Ep.IsCompleted = false
	anime.Ep.LastWasSkipped = false
	anime.Ep.Resume = false
	anime.Ep.Number = toEp
	anime.Ep.Player.PlaybackTime = 0
	anime.Ep.IsFiller = IsEpisodeFiller(anime.FillerEpisodes, toEp)

	// Episode titles / filler / recap from Jikan (non-fatal). Don't trust its duration.
	if anime.MalId > 0 {
		if err := GetEpisodeData(anime.MalId, toEp, anime); err != nil {
			Log(fmt.Sprintf("MPV playlist: GetEpisodeData ep %d: %v", toEp, err))
		}
		// Playback duration comes from MPV, not Jikan.
		anime.Ep.Duration = 0
	}
	// Filler list is authoritative when present.
	if len(anime.FillerEpisodes) > 0 {
		anime.Ep.IsFiller = IsEpisodeFiller(anime.FillerEpisodes, toEp)
	}

	// Last-played anime id (Continue Last Session).
	writeLastPlayedAnimeID(c.config.StoragePath, anime.AnilistId)

	// Local history: current episode is now toEp at position 0.
	dbPath := localHistoryPath(c.config.StoragePath)
	if dbPath != "" {
		if err := LocalUpdateAnime(
			dbPath,
			anime.AnilistId,
			anime.ProviderId,
			toEp,
			0,
			0,
			GetAnimeName(*anime),
			CurrentAnimeProviderName(anime),
		); err != nil {
			Log(fmt.Sprintf("MPV playlist: LocalUpdateAnime: %v", err))
		} else {
			Log(fmt.Sprintf("MPV playlist: last played → ep %d", toEp))
		}
	}

	// Remote progress according to leave action.
	remoteProgress := 0
	switch leave {
	case playlistLeaveMarkLeft:
		if fromEp > 0 {
			remoteProgress = fromEp
		}
	case playlistLeaveSetWatch:
		if toEp > 1 {
			remoteProgress = toEp - 1
		}
	}

	if remoteProgress > 0 {
		token := ""
		if u := GetGlobalUser(); u != nil {
			token = u.Token
		}
		if !anime.Rewatching && token != "" {
			go func(ep int) {
				if err := UpdateAnimeProgress(token, anime.AnilistId, ep); err != nil {
					Log(fmt.Sprintf("MPV playlist: UpdateAnimeProgress(%d): %v", ep, err))
				} else {
					CurdOut(fmt.Sprintf("Upstream progress → episode %d", ep))
				}
			}(remoteProgress)
		} else if anime.Rewatching {
			Log("MPV playlist: rewatching — skipped remote progress update")
		}
		CurdOut(fmt.Sprintf("Playing episode %d · upstream progress set to %d", toEp, remoteProgress))
	} else {
		CurdOut(fmt.Sprintf("Playing episode %d · last played updated (no upstream change)", toEp))
	}

	// Prefetch next episode links in preferred mode (no audio prompts).
	go c.prefetchAfterPlaylistSwitch(toEp)

	// Discord will pick up the new title/position on the next presence tick once
	// duration is known; force a soft title refresh for the window.
	title := fmt.Sprintf("%s - Episode %d", GetAnimeName(*anime), toEp)
	_, _ = MPVSendCommand(c.socket, []interface{}{"set_property", "force-media-title", title})
	_, _ = MPVSendCommand(c.socket, []interface{}{"set_property", "title", title})
}

func localHistoryPath(storagePath string) string {
	storagePath = strings.TrimSpace(os.ExpandEnv(storagePath))
	if storagePath == "" {
		return ""
	}
	return filepath.Join(storagePath, "curd_history.txt")
}

func writeLastPlayedAnimeID(storagePath string, anilistID int) {
	if anilistID <= 0 {
		return
	}
	storagePath = strings.TrimSpace(os.ExpandEnv(storagePath))
	if storagePath == "" {
		return
	}
	idPath := filepath.Join(storagePath, "curd_id")
	if err := os.MkdirAll(filepath.Dir(idPath), 0o755); err != nil {
		Log(fmt.Sprintf("MPV playlist: mkdir for curd_id: %v", err))
		return
	}
	if err := os.WriteFile(idPath, []byte(strconv.Itoa(anilistID)), 0o644); err != nil {
		Log(fmt.Sprintf("MPV playlist: write curd_id: %v", err))
	}
}

func (c *MPVPlaylistController) prefetchAfterPlaylistSwitch(currentEp int) {
	if c == nil || c.config == nil || c.anime == nil {
		return
	}
	nextEp := currentEp + 1
	if c.anime.TotalEpisodes > 0 && nextEp > c.anime.TotalEpisodes {
		return
	}
	cfg := *c.config
	cfg.SubOrDub = c.preferredMode
	next := *c.anime
	if providerNoPrefetch(CurrentAnimeProviderName(&next)) {
		c.anime.Ep.NextEpisode = NextEpisode{}
		return
	}
	result, err := ResolveEpisodeURL(cfg, &next, nextEp)
	if err != nil || len(result.Links) == 0 {
		Log(fmt.Sprintf("MPV playlist: prefetch ep %d: %v", nextEp, err))
		return
	}
	if providerNoPrefetch(result.ProviderName) {
		c.anime.Ep.NextEpisode = NextEpisode{}
		return
	}
	c.anime.Ep.NextEpisode = NextEpisode{
		Number:       nextEp,
		Links:        result.Links,
		ProviderName: result.ProviderName,
		ProviderId:   result.ProviderID,
		Mode:         result.Mode,
	}
	Log(fmt.Sprintf("MPV playlist: prefetched episode %d", nextEp))
}

func (c *MPVPlaylistController) episodeIndex(ep int, mode string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.episodeIndexLocked(ep, mode)
}

func (c *MPVPlaylistController) episodeIndexLocked(ep int, mode string) int {
	mode = normalizeTranslationType(mode)
	for i, s := range c.slots {
		if s.Episode == ep && normalizeTranslationType(s.Mode) == mode {
			return i
		}
	}
	// Fallback: first slot with that episode number.
	for i, s := range c.slots {
		if s.Episode == ep {
			return i
		}
	}
	if ep > 0 {
		return ep - 1
	}
	return 0
}

func (c *MPVPlaylistController) playSlot(slot playlistSlot) error {
	// Caller (handlePlaylistJump) already called beginMPVPlaylistSwitch.
	// We clear it only after the new stream is stably playing (or on error paths
	// that return before that — handlePlaylistJump clears on failure).
	c.suppress.Store(true)
	defer c.suppress.Store(false)

	mode := normalizeTranslationType(slot.Mode)
	if mode == "" {
		mode = c.preferredMode
	}

	cfg := *c.config
	cfg.SubOrDub = mode

	anime := c.anime
	prevEp := anime.Ep.Number
	targetEp := slot.Episode

	// Clear ALL previous-episode playback state before resolve/load so the main
	// loop cannot seek/resume the old episode into the new stream.
	anime.Ep.Resume = false
	anime.Ep.Player.PlaybackTime = 0
	anime.Ep.Duration = 0
	anime.Ep.IsCompleted = false
	anime.Ep.Number = targetEp
	anime.Ep.Links = nil
	anime.Ep.NextEpisode = NextEpisode{}
	anime.Ep.StreamReferrer = ""
	anime.Ep.SubtitleURL = ""
	anime.Ep.SkipTimes = SkipTimes{}

	Log(fmt.Sprintf("MPV playlist: resolving stream for episode %d (%s) [was %d]", targetEp, mode, prevEp))

	// Snapshot current path so we can detect a real media change after loadfile.
	oldPath := mpvStringProperty(c.socket, "path")

	// Preferred mode only — no nested DynamicSelect prompts from this goroutine.
	result, err := ResolveEpisodeURL(cfg, anime, targetEp)
	if err != nil || len(result.Links) == 0 {
		anime.Ep.Number = prevEp
		if err == nil {
			err = fmt.Errorf("no streams for episode %d", targetEp)
		}
		return err
	}

	anime.Ep.Links = result.Links
	applyStreamPlaybackHints(anime, result.Links, result.LinkHints)
	link := PrioritizeLink(result.Links)
	if link == "" {
		anime.Ep.Number = prevEp
		return fmt.Errorf("empty stream link for episode %d", targetEp)
	}

	// Refuse to "switch" to the exact same URL — that reloads the same episode.
	if oldPath != "" && (link == oldPath || strings.Contains(oldPath, link) || strings.Contains(link, oldPath)) {
		Log(fmt.Sprintf("MPV playlist: WARNING resolved link equals current path for ep %d; still forcing reload", targetEp))
	}

	title := fmt.Sprintf("%s - Episode %d", GetAnimeName(*anime), targetEp)
	if mode != "" && mode != c.preferredMode {
		title += " (" + strings.ToUpper(mode) + ")"
	}
	Log(fmt.Sprintf("MPV playlist: loading ep %d in running mpv (was path %s): %s",
		targetEp, truncateForLog(oldPath, 60), truncateForLog(link, 80)))

	anime.Ep.Player.SocketPath = c.socket
	if err := loadEpisodeInRunningMPV(c.socket, link, title, anime); err != nil {
		anime.Ep.Number = prevEp
		return err
	}

	// Wait until path actually changes away from the old media (not just time-pos).
	// WaitForMPVPlaybackStart alone is wrong — old episode still has time-pos.
	if !waitForMPVMediaChange(c.socket, oldPath, link, 30*time.Second) {
		anime.Ep.Number = prevEp
		return fmt.Errorf("episode %d did not become the active media (still on previous file?)", targetEp)
	}
	_, _ = MPVSendCommand(c.socket, []interface{}{"set_property", "time-pos", 0})
	_, _ = MPVSendCommand(c.socket, []interface{}{"set_property", "pause", false})
	_, _ = MPVSendCommand(c.socket, []interface{}{"set_property", "force-media-title", title})
	_, _ = MPVSendCommand(c.socket, []interface{}{"set_property", "title", title})

	anime.Ep.Number = targetEp
	// Keep Started=true so the main monitor stays in "watching" mode continuously.
	anime.Ep.Started = true
	anime.Ep.IsCompleted = false
	anime.Ep.Player.PlaybackTime = 0
	anime.Ep.Duration = 0
	anime.Ep.SkipTimes = SkipTimes{}
	anime.Ep.Resume = false
	// Apply provider identity from the resolve result when present.
	if result.ProviderName != "" {
		anime.ProviderName = result.ProviderName
	}
	if result.ProviderID != "" {
		anime.ProviderId = result.ProviderID
	}

	// Re-read duration for the new file (one-shot background).
	go func(socket string) {
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if c.closed() {
				return
			}
			durationPos, err := MPVSendCommand(socket, []interface{}{"get_property", "duration"})
			if err == nil && durationPos != nil {
				if duration, ok := durationPos.(float64); ok && duration > 1 {
					c.anime.Ep.Duration = int(duration + 0.5)
					Log(fmt.Sprintf("MPV playlist: episode %d duration %ds", targetEp, c.anime.Ep.Duration))
					return
				}
			}
			time.Sleep(300 * time.Millisecond)
		}
	}(c.socket)

	c.mu.Lock()
	c.currentPlaying = targetEp
	c.currentMode = mode
	// The alternate is always the *other* mode from what is now playing, so after
	// a sub→dub switch the offered extra row is sub (not dub again). Recompute
	// here so probeAndAttachAlternateAudio stays symmetric in both directions.
	c.alternateMode = alternateTranslationType(mode)
	alreadyAlt := normalizeTranslationType(slot.Mode) == c.alternateMode
	// After loadfile replace the playlist is a single entry at index 0.
	c.lastPos = 0
	c.slots = []playlistSlot{{Episode: targetEp, Mode: mode, Label: title}}
	c.hasAlternate = false
	c.mu.Unlock()

	// Stream is live — safe for the main loop again.
	endMPVPlaylistSwitch()

	// Refresh skip times in background (non-blocking for playback).
	go func(ep int) {
		if c.anime.MalId > 0 {
			if err := GetAndParseAniSkipData(c.anime.MalId, ep, 0, c.anime); err != nil {
				Log(fmt.Sprintf("AniSkip for ep %d: %v", ep, err))
			}
			if c.anime.Ep.SkipTimes.Op.Start != c.anime.Ep.SkipTimes.Op.End ||
				c.anime.Ep.SkipTimes.Ed.Start != c.anime.Ep.SkipTimes.Ed.End {
				_ = SendSkipTimesToMPV(c.anime)
			}
		}
	}(targetEp)

	// Re-probe alternate for the new episode only after placeholders are rebuilt.
	go func() {
		time.Sleep(mpvPlaylistIdleDelay + time.Second)
		if c.closed() || alreadyAlt {
			return
		}
		c.hasAlternate = false
		c.probeAndAttachAlternateAudio()
	}()

	CurdOut(fmt.Sprintf("Playing episode %d (%s)", targetEp, mode))
	return nil
}

// loadEpisodeInRunningMPV replaces the currently playing file with link.
// Note: mpv's loadfile-replace clears other playlist entries (we rebuild after).
func loadEpisodeInRunningMPV(socket, link, title string, anime *Anime) error {
	if socket == "" || link == "" {
		return fmt.Errorf("missing socket or link")
	}
	// Referrer / headers for the new stream.
	referrer := ""
	if anime != nil {
		referrer = strings.TrimSpace(anime.Ep.StreamReferrer)
		if referrer == "" {
			referrer = streamReferrerForLink(link, CurrentAnimeProviderName(anime))
		}
	}
	if referrer != "" {
		_, _ = MPVSendCommand(socket, []interface{}{"set_property", "referrer", referrer})
	}

	opts := "force-media-title=" + escapeMPVOptionValue(title)
	// Explicit replace so we never append a second "current" by accident.
	if _, err := MPVSendCommand(socket, []interface{}{"loadfile", link, "replace", 0, opts}); err != nil {
		// Fallback without options (older mpv / option parse issues).
		if _, err2 := MPVSendCommand(socket, []interface{}{"loadfile", link, "replace"}); err2 != nil {
			return fmt.Errorf("loadfile: %w", err2)
		}
		_, _ = MPVSendCommand(socket, []interface{}{"set_property", "force-media-title", title})
	}
	// Stop at start of the new file; caller unpauses after path confirms.
	_, _ = MPVSendCommand(socket, []interface{}{"set_property", "pause", true})
	_, _ = MPVSendCommand(socket, []interface{}{"set_property", "time-pos", 0})

	if anime != nil {
		sub := strings.TrimSpace(anime.Ep.SubtitleURL)
		if sub != "" {
			go func() {
				_ = waitForMPVFileReady(socket, link, 12*time.Second)
				_, _ = MPVSendCommand(socket, []interface{}{"sub-add", sub, "select"})
			}()
		}
	}
	return nil
}

func mpvStringProperty(socket, name string) string {
	v, err := MPVSendCommand(socket, []interface{}{"get_property", name})
	if err != nil || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// waitForMPVMediaChange waits until MPV's path is no longer oldPath and looks like
// the new link (or at least not the previous file / lavfi placeholder).
func waitForMPVMediaChange(socket, oldPath, newLink string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	newKey := mediaURLKey(newLink)
	oldKey := mediaURLKey(oldPath)
	Log(fmt.Sprintf("MPV playlist: waiting for media change oldKey=%q newKey=%q", truncateForLog(oldKey, 40), truncateForLog(newKey, 40)))

	for time.Now().Before(deadline) {
		path := mpvStringProperty(socket, "path")
		if path == "" {
			time.Sleep(150 * time.Millisecond)
			continue
		}
		// Still on placeholder or previous file.
		if strings.HasPrefix(path, "av://") {
			time.Sleep(150 * time.Millisecond)
			continue
		}
		if oldPath != "" && path == oldPath {
			time.Sleep(150 * time.Millisecond)
			continue
		}
		if oldKey != "" && mediaURLKey(path) == oldKey && newKey != oldKey {
			time.Sleep(150 * time.Millisecond)
			continue
		}
		// Prefer a positive match on the new URL when possible.
		if newKey != "" && (strings.Contains(path, newKey) || mediaURLKey(path) == newKey || strings.Contains(newLink, path) || strings.Contains(path, newLink)) {
			// Need a real time-pos too.
			if tpos, err := MPVSendCommand(socket, []interface{}{"get_property", "time-pos"}); err == nil && tpos != nil {
				Log(fmt.Sprintf("MPV playlist: media changed to %s", truncateForLog(path, 80)))
				return true
			}
		}
		// Path changed away from old and is not lavfi — accept.
		if oldPath != "" && path != oldPath && !strings.HasPrefix(path, "av://") {
			if tpos, err := MPVSendCommand(socket, []interface{}{"get_property", "time-pos"}); err == nil && tpos != nil {
				Log(fmt.Sprintf("MPV playlist: media path changed to %s", truncateForLog(path, 80)))
				return true
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	Log(fmt.Sprintf("MPV playlist: media change timed out; path now %q", truncateForLog(mpvStringProperty(socket, "path"), 80)))
	return false
}

// mediaURLKey extracts a stable identity from a stream URL (host + path without query).
func mediaURLKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Strip query/fragment so expiring tokens don't break comparison.
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	return raw
}

func escapeMPVOptionValue(s string) string {
	// loadfile options: quote and escape embedded quotes/backslashes.
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, `,`, `\,`)
	s = strings.ReplaceAll(s, `=`, `\=`)
	return `"` + s + `"`
}

func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// rebuildPlaylistAroundCurrent re-adds titled placeholders after loadfile replace
// wiped the playlist. Current playing entry stays index 0 until we insert before it.
func (c *MPVPlaylistController) rebuildPlaylistAroundCurrent() error {
	if c.closed() || c.anime == nil {
		return nil
	}
	episodes := c.episodeNums
	if len(episodes) == 0 {
		episodes = c.fetchEpisodeNumbers()
		c.episodeNums = episodes
	}
	if len(episodes) == 0 {
		return fmt.Errorf("no episodes for rebuild")
	}

	currentEp := c.currentPlaying
	if currentEp < 1 {
		currentEp = c.anime.Ep.Number
	}
	mode := c.currentMode
	if mode == "" {
		mode = c.preferredMode
	}

	c.suppress.Store(true)
	defer c.suppress.Store(false)
	beginMPVPlaylistSwitch()
	defer endMPVPlaylistSwitch()

	var before, after []playlistSlot
	for _, ep := range episodes {
		if ep < currentEp {
			before = append(before, playlistSlot{Episode: ep, Mode: mode, Label: c.episodeLabel(ep, mode)})
		} else if ep > currentEp {
			after = append(after, playlistSlot{Episode: ep, Mode: mode, Label: c.episodeLabel(ep, mode)})
		}
	}

	// Title the live entry (index 0 after replace).
	curLabel := c.episodeLabel(currentEp, mode)
	_, _ = MPVSendCommand(c.socket, []interface{}{"set_property", "force-media-title", curLabel})
	_, _ = MPVSendCommand(c.socket, []interface{}{"set_property", "title", curLabel})

	if len(after) > 0 {
		if err := c.loadTitledPlaceholders(after, false); err != nil {
			Log(fmt.Sprintf("rebuild append future: %v", err))
		}
	}
	if len(before) > 0 {
		if err := c.loadTitledPlaceholders(before, true); err != nil {
			Log(fmt.Sprintf("rebuild insert previous: %v", err))
		}
	}

	slots := make([]playlistSlot, 0, len(before)+1+len(after))
	slots = append(slots, before...)
	slots = append(slots, playlistSlot{Episode: currentEp, Mode: mode, Label: curLabel})
	slots = append(slots, after...)

	c.mu.Lock()
	c.slots = slots
	c.lastPos = len(before)
	c.mu.Unlock()

	Log(fmt.Sprintf("MPV playlist rebuilt around ep %d (%d entries)", currentEp, len(slots)))
	return nil
}

