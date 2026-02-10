package names

import (
	"regexp"
	"testing"
)

func TestSpriteNames_Unique(t *testing.T) {
	seen := make(map[string]struct{}, len(SpriteNames))
	for _, name := range SpriteNames {
		if _, ok := seen[name]; ok {
			t.Fatalf("duplicate name in SpriteNames: %q", name)
		}
		seen[name] = struct{}{}
	}
}

func TestSpriteNames_AreValidDNSLabels(t *testing.T) {
	// Project requirement: lowercase alpha only.
	re := regexp.MustCompile(`^[a-z]+$`)
	for _, name := range SpriteNames {
		if name == "" {
			t.Fatalf("empty name in SpriteNames")
		}
		if len(name) > 63 {
			t.Fatalf("name %q is too long for a DNS label (%d > 63)", name, len(name))
		}
		if !re.MatchString(name) {
			t.Fatalf("name %q is not a valid lowercase alpha DNS label", name)
		}
	}
}

func TestPickName_WithinPool(t *testing.T) {
	if Count() != 40 {
		t.Fatalf("Count()=%d, want 40", Count())
	}

	for i := 0; i < Count(); i++ {
		got := PickName(i)
		want := SpriteNames[i]
		if got != want {
			t.Fatalf("PickName(%d)=%q, want %q", i, got, want)
		}
	}
}

func TestPickName_WrapsWithSuffix(t *testing.T) {
	if Count() != 40 {
		t.Fatalf("Count()=%d, want 40", Count())
	}

	tests := []struct {
		index int
		want  string
	}{
		{40, "bramble-2"},
		{41, "fern-2"},
		{79, "tansy-2"},
		{80, "bramble-3"},
		{81, "fern-3"},
	}

	for _, tt := range tests {
		if got := PickName(tt.index); got != tt.want {
			t.Fatalf("PickName(%d)=%q, want %q", tt.index, got, tt.want)
		}
	}
}

func TestNameIndex_RoundTrip(t *testing.T) {
	for i, name := range SpriteNames {
		if got := NameIndex(name); got != i {
			t.Fatalf("NameIndex(%q)=%d, want %d", name, got, i)
		}
	}
}

func TestNameIndex_Unknown(t *testing.T) {
	if got := NameIndex("unknown"); got != -1 {
		t.Fatalf("NameIndex(%q)=%d, want -1", "unknown", got)
	}
	if got := NameIndex("bramble-2"); got != -1 {
		t.Fatalf("NameIndex(%q)=%d, want -1 (suffixes not supported)", "bramble-2", got)
	}
}
