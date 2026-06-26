package internal

import (
	"path/filepath"
	"strings"
)

var curdVersion = "dev"

// SetCurdVersion records the running application version for storage migrations.
func SetCurdVersion(version string) {
	version = strings.TrimSpace(version)
	if version != "" {
		curdVersion = version
	}
}

// CurdVersion returns the running application version.
func CurdVersion() string {
	if curdVersion == "" {
		return "dev"
	}
	return curdVersion
}

func storageVersionFilePath(storagePath string) string {
	return filepath.Join(strings.TrimSpace(storagePath), "curd_version")
}
