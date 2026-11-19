package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Live mpv verification for the frozen playlist queue. Run with:
//
//	CURD_LIVE_MPV=1 go test ./internal/ -run TestLivePlaylistFrozenQueue -v -timeout 5m
//
// It pins the reported defect class against real mpv: after build and after
// a loadfile-replace collapse + synchronous rebuild, every queue position
// must resolve to the episode the frozen order says — never a stale title.
func TestLivePlaylistFrozenQueue(t *testing.T) {
	if os.Getenv("CURD_LIVE_MPV") == "" {
		t.Skip("set CURD_LIVE_MPV=1 to run live mpv playlist tests")
	}

	sock := filepath.Join(t.TempDir(), "mpv.sock")
	cmd := exec.Command("mpv", "--idle=yes", "--vo=null", "--ao=null",
		"--input-ipc-server="+sock, "--no-terminal")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start mpv: %v", err)
	}
	defer func() {
		_, _ = MPVSendCommand(sock, []interface{}{"quit"})
		_ = cmd.Wait()
	}()

	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := MPVSendCommand(sock, []interface{}{"get_property", "mpv-version"}); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("mpv socket never came up")
		}
		time.Sleep(200 * time.Millisecond)
	}

	loadLavfi := func() {
		t.Helper()
		if _, err := MPVSendCommand(sock, []interface{}{"loadfile", "av://lavfi:color=c=blue:s=16x16:d=86400", "replace"}); err != nil {
			t.Fatalf("loadfile: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	loadLavfi()

	newController := func(current int) *MPVPlaylistController {
		return &MPVPlaylistController{
			config:         &CurdConfig{StoragePath: t.TempDir()},
			anime:          &Anime{Ep: Episode{Number: current}, TotalEpisodes: 5},
			socket:         sock,
			done:           make(chan struct{}),
			lastPos:        -1,
			preferredMode:  "sub",
			currentPlaying: current,
			currentMode:    "sub",
		}
	}
	playlistPlumbing := func() []string {
		t.Helper()
		v, err := MPVSendCommand(sock, []interface{}{"get_property", "playlist"})
		if err != nil {
			t.Fatalf("get playlist: %v", err)
		}
		arr, ok := v.([]interface{})
		if !ok {
			t.Fatalf("playlist not an array: %T", v)
		}
		var out []string
		for i := range arr {
			f, _ := MPVSendCommand(sock, []interface{}{"get_property", fmt.Sprintf("playlist/%d/filename", i)})
			ti, _ := MPVSendCommand(sock, []interface{}{"get_property", fmt.Sprintf("playlist/%d/title", i)})
			out = append(out, fmt.Sprintf("pos=%d file=%v title=%v", i, f, ti))
		}
		return out
	}
	checkMapping := func(c *MPVPlaylistController, want []int) {
		t.Helper()
		for pos, ep := range want {
			slot, err := c.resolveSlotAt(pos)
			if err != nil {
				t.Fatalf("pos %d: resolve: %v\nqueue:\n%s", pos, err, joinLines(playlistPlumbing()))
			}
			if slot.Episode != ep {
				t.Fatalf("pos %d: got ep %d want ep %d\nqueue:\n%s", pos, slot.Episode, ep, joinLines(playlistPlumbing()))
			}
		}
	}

	// Ep 3 of 5: queue must be [1 2 3 4 5] with pos on 3.
	c := newController(3)
	if err := c.buildEpisodePlaylist(); err != nil {
		t.Fatalf("build: %v\nqueue:\n%s", err, joinLines(playlistPlumbing()))
	}
	checkMapping(c, []int{1, 2, 3, 4, 5})
	if pos, err := c.playlistPos(); err != nil || pos != 2 {
		t.Fatalf("pos=%d err=%v want 2\nqueue:\n%s", pos, err, joinLines(playlistPlumbing()))
	}

	// Simulate what every episode load does (loadfile replace collapses the
	// queue), then rebuild synchronously like the jump path does.
	loadLavfi()
	c.mu.Lock()
	c.currentPlaying = 4
	c.slots = []playlistSlot{{Episode: 4, Mode: "sub", Label: "x"}}
	c.lastPos = 0
	c.mu.Unlock()
	if err := c.rebuildPlaylistAroundCurrent(); err != nil {
		t.Fatalf("rebuild: %v\nqueue:\n%s", err, joinLines(playlistPlumbing()))
	}
	checkMapping(c, []int{1, 2, 3, 4, 5})
	if pos, err := c.playlistPos(); err != nil || pos != 3 {
		t.Fatalf("after rebuild pos=%d err=%v want 3\nqueue:\n%s", pos, err, joinLines(playlistPlumbing()))
	}

	// Single-episode show: fresh player means the queue is just the live
	// entry (loadfile replace collapses, exactly like a new mpv process).
	loadLavfi()
	single := newController(1)
	single.anime.TotalEpisodes = 1
	if err := single.buildEpisodePlaylist(); err != nil {
		t.Fatalf("single build: %v", err)
	}
	checkMapping(single, []int{1})
}

func joinLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += "  " + l + "\n"
	}
	return out
}
