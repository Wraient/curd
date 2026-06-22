package providers

import "strings"

func NormalizeTranslationType(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "dub") {
		return "dub"
	}
	return "sub"
}

func AlternateTranslationType(mode string) string {
	if NormalizeTranslationType(mode) == "dub" {
		return "sub"
	}
	return "dub"
}
