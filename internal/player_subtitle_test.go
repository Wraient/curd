package internal

import (
	"errors"
	"testing"
	"time"
)

func TestWaitForMPVFileReadyRetriesUntilPropertiesExist(t *testing.T) {
	pathCalls := 0
	sender := func(_ string, command []interface{}) (interface{}, error) {
		switch command[1] {
		case "path":
			pathCalls++
			if pathCalls < 3 {
				return nil, errors.New("property unavailable")
			}
			return "https://cdn.example/episode.m3u8", nil
		case "duration":
			return float64(1440), nil
		default:
			return nil, errors.New("unexpected command")
		}
	}
	if err := waitForMPVFileReadyWith(sender, "socket", "https://cdn.example/episode.m3u8", 100*time.Millisecond, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if pathCalls != 3 {
		t.Fatalf("path queried %d times, want 3", pathCalls)
	}
}
