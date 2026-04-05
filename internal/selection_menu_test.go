package internal

import (
	"strings"
	"testing"
)

func TestAndroidSelectionViewShowsMobileFooter(t *testing.T) {
	SetGlobalConfig(&CurdConfig{Platform: string(PlatformAndroid)})

	model := Model{
		allOptions: []SelectionOption{
			{Key: "1", Label: "One Piece"},
			{Key: "2", Label: "Frieren"},
		},
		filteredKeys: []SelectionOption{
			{Key: "1", Label: "One Piece"},
			{Key: "2", Label: "Frieren"},
		},
		terminalHeight: 20,
		isHomeMenu:     false,
	}

	view := model.View()
	if !strings.Contains(view, "Enter select  Esc back  Ctrl+C quit  Type to search") {
		t.Fatalf("expected android footer in view: %s", view)
	}
	if !strings.Contains(view, "Search Anime") {
		t.Fatalf("expected android title in view: %s", view)
	}
}
