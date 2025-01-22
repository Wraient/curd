//go:build windows
// +build windows

package internal

import (
	"net"

	"github.com/Microsoft/go-winio"
)

func connectToPipe(ipcSocketPath string) (net.Conn, error) {
	conn, err := winio.DialPipe(ipcSocketPath, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
