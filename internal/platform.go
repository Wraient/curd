package internal

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Platform string

const (
	PlatformDesktop Platform = "desktop"
	PlatformAndroid Platform = "android"
)

func DetectPlatform() Platform {
	if runtime.GOOS == "android" {
		return PlatformAndroid
	}

	termuxMarkers := []string{
		os.Getenv("TERMUX_VERSION"),
		os.Getenv("TERMUX_APP_PID"),
	}
	for _, marker := range termuxMarkers {
		if strings.TrimSpace(marker) != "" {
			return PlatformAndroid
		}
	}

	prefix := os.Getenv("PREFIX")
	if strings.Contains(prefix, "com.termux") {
		return PlatformAndroid
	}

	return PlatformDesktop
}

func DefaultConfigPathForPlatform(platform Platform) string {
	homeDir := os.Getenv("HOME")
	if runtime.GOOS == "windows" && homeDir == "" {
		homeDir = os.Getenv("USERPROFILE")
	}

	fileName := "curd.conf"
	if platform == PlatformAndroid {
		fileName = "android.conf"
	}

	return filepath.Join(homeDir, ".config", "curd", fileName)
}

func DefaultConfigPath() string {
	return DefaultConfigPathForPlatform(DetectPlatform())
}

func InferPlatformForConfigPath(configPath string, detected Platform) Platform {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(configPath)))
	switch base {
	case "android.conf":
		return PlatformAndroid
	case "curd.conf":
		return PlatformDesktop
	default:
		return detected
	}
}
