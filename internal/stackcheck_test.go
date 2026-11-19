package internal

import (
	"reflect"
	"testing"
)

func TestDefaultStackIsAnipmAnikotoAnilightAnimexAniwavesAnipub(t *testing.T) {
	got := defaultEnabledProviderStack()
	want := []string{"anipm", "anikoto", "anilight", "animex", "aniwaves", "anipub"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default stack = %v, want %v", got, want)
	}
	// Senshi is default-disabled only while its upstream is down (see its
	// register.go); re-add it here when re-enabled.
	for _, name := range []string{"senshi", "anineko", "allanime", "animepahe"} {
		if ProviderEnabled(name) {
			t.Fatalf("%s should be disabled by default", name)
		}
		if ProviderDisabledReason(name) == "" {
			t.Fatalf("%s should have a disable reason", name)
		}
	}
}
