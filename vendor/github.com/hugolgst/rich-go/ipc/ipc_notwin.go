// +build !windows

package ipc

import (
	"net"
	"time"
)

// OpenSocket opens the discord-ipc-0 unix socket
func OpenSocket() error {
	sock, err := net.DialTimeout("unix", GetIpcPath()+"/discord-ipc-0", time.Second*2)
	if err != nil {
		return err
	}

	socket = sock
	return nil
}
