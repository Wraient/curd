package internal

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Tracker progress writes used to be fire-and-forget goroutines. Two bugs
// came with that:
//   - lost writes: `go UpdateAnimeProgress(...)` followed by ExitCurd →
//     os.Exit killed the goroutine before the request finished, so the
//     tracker never saw the episode the user just completed;
//   - reordering: back-to-back episodes raced, and "ep 5" landing after
//     "ep 6" left the tracker behind.
// The queue keeps writes off the playback path but runs them one at a time,
// in order, and exitWithRestore flushes it (bounded) before the process dies.

type progressJob struct {
	token    string
	mediaID  int
	progress int
	// exact jobs are explicit user choices (e.g. jumping back in the mpv
	// playlist) and may lower progress. Automatic completion writes are
	// monotonic: they never move the tracker below its known progress.
	exact bool
	// rewatching is snapshotted at enqueue (caller's goroutine), so the
	// worker never reads the live anime struct.
	rewatching bool
	onResult   func(error)
}

var progressQueue struct {
	mu      sync.Mutex
	jobs    []progressJob
	running bool
}

// progressUpdater is the write function; a var so tests can stub it.
var progressUpdater = UpdateAnimeProgress

// progressFlushTimeout bounds how long exit waits for queued writes. A
// tracker that is down must not hold the terminal hostage.
var progressFlushTimeout = 10 * time.Second

// progressRateLimitRetries/Delay requeue a write the tracker rate limited
// (MAL has no in-client 429 retry). The worker is off the playback path.
var progressRateLimitRetries = 3
var progressRateLimitDelay = 20 * time.Second

// remoteProgressFor reports the tracker's known progress for a show from
// the (background-refreshed) list. A var so tests can stub it.
var remoteProgressFor = func(mediaID int) (progress int, repeating bool, ok bool) {
	u := GetGlobalUser()
	if u == nil {
		return 0, false, false
	}
	list := u.AnimeList
	if u.ListSync != nil {
		list = u.ListSync.Current()
	}
	entry, err := FindAnimeByAnilistID(list, strconv.Itoa(mediaID))
	if err != nil || entry == nil {
		return 0, false, false
	}
	return entry.Progress, entry.Status == "REPEATING", true
}

// QueueAnimeProgress schedules an automatic (monotonic) tracker progress
// write without blocking the caller. Writes run sequentially in submission
// order. onResult, if non-nil, runs on the worker goroutine afterwards.
func QueueAnimeProgress(token string, mediaID, progress int, onResult func(error)) {
	enqueueProgress(progressJob{token: token, mediaID: mediaID, progress: progress, onResult: onResult})
}

// QueueAnimeProgressExact schedules a user-chosen progress value, which
// may be lower than the tracker's current progress.
func QueueAnimeProgressExact(token string, mediaID, progress int, onResult func(error)) {
	enqueueProgress(progressJob{token: token, mediaID: mediaID, progress: progress, exact: true, onResult: onResult})
}

func enqueueProgress(job progressJob) {
	if a := GetGlobalAnime(); a != nil && a.AnilistId == job.mediaID && a.Rewatching {
		job.rewatching = true
	}
	if job.mediaID <= 0 || job.progress < 0 {
		if job.onResult != nil {
			job.onResult(fmt.Errorf("invalid progress write (media %d, episode %d)", job.mediaID, job.progress))
		}
		return
	}
	progressQueue.mu.Lock()
	progressQueue.jobs = append(progressQueue.jobs, job)
	if !progressQueue.running {
		progressQueue.running = true
		go runProgressQueue()
	}
	progressQueue.mu.Unlock()
}

func runProgressQueue() {
	for {
		progressQueue.mu.Lock()
		if len(progressQueue.jobs) == 0 {
			progressQueue.running = false
			progressQueue.mu.Unlock()
			return
		}
		job := progressQueue.jobs[0]
		progressQueue.jobs = progressQueue.jobs[1:]
		progressQueue.mu.Unlock()

		err := runProgressJob(job)
		if err != nil {
			Log(fmt.Sprintf("Tracker progress update (media %d → ep %d) failed: %v", job.mediaID, job.progress, err))
		}
		if job.onResult != nil {
			job.onResult(err)
		}
	}
}

func runProgressJob(job progressJob) error {
	if !job.exact && !job.rewatching {
		// Rewatch progress legitimately restarts below the old count.
		if remote, repeating, ok := remoteProgressFor(job.mediaID); ok && !repeating && remote > job.progress {
			// The tracker is already ahead (watched elsewhere, or the list
			// refreshed after playback started): an automatic write would
			// move it backwards.
			Log(fmt.Sprintf("Skipping tracker write for media %d: tracker already at %d (> %d)", job.mediaID, remote, job.progress))
			return nil
		}
	}
	var err error
	for attempt := 0; ; attempt++ {
		err = safeProgressUpdate(job)
		if err == nil || !isRateLimitError(err) || attempt >= progressRateLimitRetries || ExitInProgress() {
			return err
		}
		time.Sleep(progressRateLimitDelay)
	}
}

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "status 429") || strings.Contains(msg, "Too Many Requests") || strings.Contains(msg, "too_many_requests")
}

func safeProgressUpdate(job progressJob) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("progress update panicked: %v", r)
		}
	}()
	return progressUpdater(job.token, job.mediaID, job.progress)
}

// PendingProgressWrites reports queued plus in-flight writes.
func PendingProgressWrites() int {
	progressQueue.mu.Lock()
	defer progressQueue.mu.Unlock()
	n := len(progressQueue.jobs)
	if progressQueue.running {
		n++
	}
	return n
}

// FlushProgressQueue waits until every queued write finished or timeout
// elapses, and reports whether the queue drained. Polls rather than
// parking on a condition so a timed-out flush leaves nothing behind.
func FlushProgressQueue(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if PendingProgressWrites() == 0 {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// UpdateAnimeProgressOrdered writes progress synchronously but behind any
// queued writes, so a direct write can never race ahead of an older queued
// one and leave the tracker behind. Waits at most timeout for its result;
// on timeout the write stays queued and still lands, in order.
func UpdateAnimeProgressOrdered(token string, mediaID, progress int, timeout time.Duration) error {
	result := make(chan error, 1)
	QueueAnimeProgress(token, mediaID, progress, func(err error) { result <- err })
	select {
	case err := <-result:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("tracker progress update timed out after %s (still queued)", timeout)
	}
}
