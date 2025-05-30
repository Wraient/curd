//go:build !windows
// +build !windows

package internal

import (
	"net"
)

func connectToPipe(ipcSocketPath string) (net.Conn, error) {
	conn, err := net.Dial("unix", ipcSocketPath)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
