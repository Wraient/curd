package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigForPlatformCreatesAndroidSchema(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath := filepath.Join(os.Getenv("HOME"), ".config", "curd", "android.conf")
	config, err := LoadConfigForPlatform(configPath, PlatformAndroid)
	if err != nil {
		t.Fatalf("LoadConfigForPlatform returned error: %v", err)
	}

	if !config.IsAndroid() {
		t.Fatalf("expected android platform config, got %+v", config)
	}
	if config.AndroidPlayerPackage == "" {
		t.Fatalf("expected AndroidPlayerPackage to be populated")
	}
	if config.Player != "" {
		t.Fatalf("expected desktop Player to be excluded from android schema, got %q", config.Player)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "AndroidPlayerPackage=") {
		t.Fatalf("expected AndroidPlayerPackage in android config: %s", content)
	}
	if strings.Contains(content, "Player=") {
		t.Fatalf("did not expect desktop Player key in android config: %s", content)
	}
}

func TestLoadConfigForPlatformCreatesDesktopSchema(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath := filepath.Join(os.Getenv("HOME"), ".config", "curd", "curd.conf")
	config, err := LoadConfigForPlatform(configPath, PlatformDesktop)
	if err != nil {
		t.Fatalf("LoadConfigForPlatform returned error: %v", err)
	}

	if config.IsAndroid() {
		t.Fatalf("expected desktop platform config, got %+v", config)
	}
	if config.Player == "" {
		t.Fatalf("expected desktop Player to be populated")
	}
	if config.AndroidPlayerPackage != "" {
		t.Fatalf("expected AndroidPlayerPackage to be excluded from desktop schema, got %q", config.AndroidPlayerPackage)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Player=") {
		t.Fatalf("expected Player in desktop config: %s", content)
	}
	if strings.Contains(content, "AndroidPlayerPackage=") {
		t.Fatalf("did not expect AndroidPlayerPackage in desktop config: %s", content)
	}
}

func TestDefaultConfigPathForPlatform(t *testing.T) {
	t.Setenv("HOME", "/tmp/curd-home")

	androidPath := DefaultConfigPathForPlatform(PlatformAndroid)
	desktopPath := DefaultConfigPathForPlatform(PlatformDesktop)

	if !strings.HasSuffix(androidPath, filepath.Join(".config", "curd", "android.conf")) {
		t.Fatalf("unexpected android config path: %s", androidPath)
	}
	if !strings.HasSuffix(desktopPath, filepath.Join(".config", "curd", "curd.conf")) {
		t.Fatalf("unexpected desktop config path: %s", desktopPath)
	}
}

func TestInferPlatformForConfigPath(t *testing.T) {
	if got := InferPlatformForConfigPath("/tmp/android.conf", PlatformDesktop); got != PlatformAndroid {
		t.Fatalf("expected android platform for android.conf, got %q", got)
	}
	if got := InferPlatformForConfigPath("/tmp/curd.conf", PlatformAndroid); got != PlatformDesktop {
		t.Fatalf("expected desktop platform for curd.conf, got %q", got)
	}
}
