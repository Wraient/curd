package internal

import (
	"reflect"
	"testing"
)

func TestDefaultStackIsSenshiAnipub(t *testing.T) {
	got := defaultEnabledProviderStack()
	want := []string{"senshi", "anipub"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default stack = %v, want %v", got, want)
	}
	for _, name := range []string{"anineko", "allanime", "animepahe"} {
		if ProviderEnabled(name) {
			t.Fatalf("%s should be disabled by default", name)
		}
		if ProviderDisabledReason(name) == "" {
			t.Fatalf("%s should have a disable reason", name)
		}
	}
}
