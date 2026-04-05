package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildAndroidIntentCommandUsesConfiguredComponentAndPlaceholders(t *testing.T) {
	config := &CurdConfig{
		Platform:                  string(PlatformAndroid),
		AndroidPlayerPackage:      "is.xyz.mpv",
		AndroidPlayerActivity:     ".MPVActivity",
		AndroidPlayerIntentAction: "android.intent.action.VIEW",
		AndroidExtraIntentArgs: []string{
			"--es",
			"title",
			"{title}",
			"--es",
			"socket",
			"{socket}",
		},
	}

	cmd := BuildAndroidIntentCommand(config, "https://example.com/video.mp4", "Episode 3", "/tmp/test.sock")
	joined := strings.Join(cmd, " ")

	if !strings.Contains(joined, "-n is.xyz.mpv/.MPVActivity") {
		t.Fatalf("expected component in command: %s", joined)
	}
	if !strings.Contains(joined, "https://example.com/video.mp4") {
		t.Fatalf("expected URL placeholder to be replaced: %s", joined)
	}
	if !strings.Contains(joined, "Episode 3") {
		t.Fatalf("expected title placeholder to be replaced: %s", joined)
	}
	if !strings.Contains(joined, "/tmp/test.sock") {
		t.Fatalf("expected socket placeholder to be replaced: %s", joined)
	}
}

func TestProbeAndroidPlayerReturnsUnsupportedWhenPackageMissing(t *testing.T) {
	config := &CurdConfig{
		Platform:              string(PlatformAndroid),
		AndroidPlayerPackage:  "missing.package",
		AndroidPlayerActivity: ".MissingActivity",
		AndroidPlayerMode:     "ipc",
	}

	result := ProbeAndroidPlayer(config)

	if result.Capability != "unsupported" {
		t.Fatalf("expected unsupported capability, got %q", result.Capability)
	}
	if len(result.Warnings) == 0 {
		t.Fatalf("expected warning when package probe fails")
	}
}

func TestAndroidBackendSupportsAutomationOnlyInIPCMode(t *testing.T) {
	intentBackend := &androidPlayerBackend{config: &CurdConfig{
		Platform:          string(PlatformAndroid),
		AndroidPlayerMode: "intent",
	}}
	if intentBackend.SupportsAutomation() {
		t.Fatalf("intent backend should not support automation")
	}

	ipcBackend := &androidPlayerBackend{config: &CurdConfig{
		Platform:          string(PlatformAndroid),
		AndroidPlayerMode: "ipc",
	}}
	if !ipcBackend.SupportsAutomation() {
		t.Fatalf("ipc backend should support automation")
	}
}

func TestAndroidConfiguredSocketPathUsesStaticPathWhenProvided(t *testing.T) {
	config := &CurdConfig{
		Platform:                string(PlatformAndroid),
		AndroidPlayerSocketPath: "/data/data/com.termux/files/home/.tmp/mpvex.sock",
		AndroidExtraIntentArgs:  []string{"--es", "ignored", "{socket}"},
	}

	if got := androidConfiguredSocketPath(config); got != "/data/data/com.termux/files/home/.tmp/mpvex.sock" {
		t.Fatalf("expected static socket path, got %q", got)
	}
}

func TestAndroidConfiguredSocketPathUsesPlaceholderWhenConfigured(t *testing.T) {
	config := &CurdConfig{
		Platform:               string(PlatformAndroid),
		AndroidExtraIntentArgs: []string{"--es", "socket", "{socket}"},
	}

	if got := androidConfiguredSocketPath(config); got == "" {
		t.Fatalf("expected generated socket path when {socket} placeholder is present")
	}
}

func TestAssessAndroidSocketPathRejectsTermuxPrivateStorage(t *testing.T) {
	t.Setenv("HOME", filepath.Join(string(os.PathSeparator), "data", "data", "com.termux", "files", "home"))

	result := assessAndroidSocketPath("/data/data/com.termux/files/home/.tmp/mpv.sock")
	if result.Supported {
		t.Fatalf("expected Termux private storage socket path to be rejected")
	}
	if len(result.Warnings) == 0 {
		t.Fatalf("expected warning for Termux private storage socket path")
	}
}

func TestAssessAndroidSocketPathRejectsSharedStorage(t *testing.T) {
	result := assessAndroidSocketPath("/storage/emulated/0/Download/mpv.sock")
	if result.Supported {
		t.Fatalf("expected shared storage socket path to be rejected")
	}
	if len(result.Warnings) == 0 {
		t.Fatalf("expected warning for shared storage socket path")
	}
}
