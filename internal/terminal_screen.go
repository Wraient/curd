package internal

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	interruptHandlerOnce sync.Once
	interruptExitOnce    sync.Once
)

// InstallTerminalInterruptHandler restores the terminal before exiting on Ctrl+C or SIGTERM.
func InstallTerminalInterruptHandler() {
	interruptHandlerOnce.Do(func() {
		interrupts := make(chan os.Signal, 1)
		signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)
		go func() {
			first := true
			for range interrupts {
				if !first {
					// Second Ctrl+C while flushing tracker writes: leave now.
					RestoreScreen()
					os.Exit(130)
				}
				first = false
				go exitWithRestore(130)
			}
		}()
	})
}

// processExiting is set once shutdown begins. While exit flushes tracker
// writes the main loop is still running (and mpv may have died from the
// same Ctrl+C), so playback/menu entry points park instead of launching a
// new episode that os.Exit would orphan.
var processExiting atomic.Bool

// ExitInProgress reports whether the process is shutting down.
func ExitInProgress() bool { return processExiting.Load() }

// parkIfExiting blocks the caller forever once shutdown has begun; the
// pending os.Exit ends the process.
func parkIfExiting() {
	if processExiting.Load() {
		select {}
	}
}

func exitWithRestore(code int) {
	interruptExitOnce.Do(func() {
		processExiting.Store(true)
		RestoreScreen()
		if code == 130 {
			// Ctrl+C: make sure the player goes too.
			if a := GetGlobalAnime(); a != nil && a.Ep.Player.SocketPath != "" && a.Ep.Player.SocketPath != "android-intent" {
				_, _ = MPVSendCommand(a.Ep.Player.SocketPath, []interface{}{"quit"})
			}
		}
		// Queued tracker writes would die with the process; give them a
		// bounded moment (shorter when the user interrupted).
		if PendingProgressWrites() > 0 {
			timeout := progressFlushTimeout
			if code == 130 {
				timeout = 3 * time.Second
			}
			fmt.Fprintln(os.Stderr, "Saving tracker progress...")
			if !FlushProgressQueue(timeout) {
				fmt.Fprintln(os.Stderr, "Tracker did not respond in time; local history is saved.")
			}
		}
		os.Exit(code)
	})
}
