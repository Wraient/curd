//go:build windows
// +build windows

package ipc

import (
	"time"

	npipe "gopkg.in/natefinch/npipe.v2"
)

// OpenSocket opens the discord-ipc-0 named pipe
func OpenSocket() error {
	// Connect to the Windows named pipe, this is a well known name
	// We use DialTimeout since it will block forever (or very very long) on Windows
	// if the pipe is not available (Discord not running)
	sock, err := npipe.DialTimeout(`\\.\pipe\discord-ipc-0`, time.Second*2)
	if err != nil {
		return err
	}

	socket = sock
	return nil
}
