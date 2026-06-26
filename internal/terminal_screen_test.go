package internal

import "testing"

func TestRestoreScreenOnlyWhenAlternateScreenActive(t *testing.T) {
	previous := alternateScreenActive
	t.Cleanup(func() {
		alternateScreenActive = previous
	})

	alternateScreenActive = false
	RestoreScreen()
	if alternateScreenActive {
		t.Fatal("restore should stay inactive when screen was not cleared")
	}

	alternateScreenActive = true
	RestoreScreen()
	if alternateScreenActive {
		t.Fatal("restore should deactivate alternate screen")
	}
}

func TestInstallTerminalInterruptHandlerIsIdempotent(t *testing.T) {
	InstallTerminalInterruptHandler()
	InstallTerminalInterruptHandler()
}
