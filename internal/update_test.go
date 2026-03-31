package internal

import "testing"

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain semver", input: "1.2.3", want: "1.2.3"},
		{name: "trim leading v", input: "v1.2.3", want: "1.2.3"},
		{name: "trim whitespace", input: "  v2.0.1  ", want: "2.0.1"},
		{name: "development build", input: "(devel)", want: ""},
		{name: "invalid", input: "latest", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeVersion(test.input); got != test.want {
				t.Fatalf("normalizeVersion(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  int
	}{
		{name: "equal", left: "1.2.3", right: "1.2.3", want: 0},
		{name: "patch newer", left: "1.2.4", right: "1.2.3", want: 1},
		{name: "minor older", left: "1.1.9", right: "1.2.0", want: -1},
		{name: "prefix ignored", left: "v1.3.0", right: "1.2.9", want: 1},
		{name: "missing patch treated as zero", left: "1.2", right: "1.2.0", want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := compareVersions(test.left, test.right); got != test.want {
				t.Fatalf("compareVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
			}
		})
	}
}
