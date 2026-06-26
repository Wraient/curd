package internal

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
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
			for range interrupts {
				exitWithRestore(130)
			}
		}()
	})
}

func exitWithRestore(code int) {
	interruptExitOnce.Do(func() {
		RestoreScreen()
		os.Exit(code)
	})
}
