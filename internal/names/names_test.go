package names

import (
	"regexp"
	"testing"
)

func TestSpriteNames_Unique(t *testing.T) {
	all := AllNames()
	seen := make(map[string]struct{}, len(all))
	for _, name := range all {
		if _, ok := seen[name]; ok {
			t.Fatalf("duplicate name: %q", name)
		}
		seen[name] = struct{}{}
	}
}

func TestSpriteNames_Immutable(t *testing.T) {
	// AllNames returns a copy — mutating it must not affect the pool.
	all := AllNames()
	all[0] = "MUTATED"
	fresh := AllNames()
	if fresh[0] == "MUTATED" {
		t.Fatal("AllNames() returned a reference, not a copy — pool is mutable")
	}
}

func TestSpriteNames_AreValidDNSLabels(t *testing.T) {
	re := regexp.MustCompile(`^[a-z]+$`)
	for _, name := range AllNames() {
		if name == "" {
			t.Fatalf("empty name in pool")
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

	all := AllNames()
	for i := 0; i < Count(); i++ {
		got, err := PickName(i)
		if err != nil {
			t.Fatalf("PickName(%d) error = %v", i, err)
		}
		if got != all[i] {
			t.Fatalf("PickName(%d)=%q, want %q", i, got, all[i])
		}
	}
}

func TestPickName_WrapsWithSuffix(t *testing.T) {
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
		got, err := PickName(tt.index)
		if err != nil {
			t.Fatalf("PickName(%d) error = %v", tt.index, err)
		}
		if got != tt.want {
			t.Fatalf("PickName(%d)=%q, want %q", tt.index, got, tt.want)
		}
	}
}

func TestPickName_NegativeIndex(t *testing.T) {
	_, err := PickName(-1)
	if err == nil {
		t.Fatal("expected error for negative index")
	}
}

func TestNameIndex_BaseNames(t *testing.T) {
	all := AllNames()
	for i, name := range all {
		if got := NameIndex(name); got != i {
			t.Fatalf("NameIndex(%q)=%d, want %d", name, got, i)
		}
	}
}

func TestNameIndex_SuffixedNames(t *testing.T) {
	// NameIndex now handles suffixed names — round-trip property holds.
	tests := []struct {
		name string
		want int
	}{
		{"bramble-2", 40},
		{"fern-2", 41},
		{"tansy-2", 79},
		{"bramble-3", 80},
	}

	for _, tt := range tests {
		if got := NameIndex(tt.name); got != tt.want {
			t.Fatalf("NameIndex(%q)=%d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestNameIndex_RoundTrip(t *testing.T) {
	// Verify round-trip: NameIndex(PickName(i)) == i for any valid i.
	for i := 0; i < Count()*3; i++ {
		name, err := PickName(i)
		if err != nil {
			t.Fatalf("PickName(%d) error = %v", i, err)
		}
		if got := NameIndex(name); got != i {
			t.Fatalf("round-trip failed: PickName(%d)=%q, NameIndex(%q)=%d", i, name, name, got)
		}
	}
}

func TestNameIndex_Unknown(t *testing.T) {
	if got := NameIndex("unknown"); got != -1 {
		t.Fatalf("NameIndex(%q)=%d, want -1", "unknown", got)
	}
	if got := NameIndex(""); got != -1 {
		t.Fatalf("NameIndex(%q)=%d, want -1", "", got)
	}
}
