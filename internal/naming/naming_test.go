package naming

import (
	"errors"
	"testing"
)

func TestPickNext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		excluded      []string
		wantErr       error
		validatePick  func(t *testing.T, picked string)
	}{
		{
			name:     "no exclusions returns first name",
			excluded: nil,
			wantErr:  nil,
			validatePick: func(t *testing.T, picked string) {
				if picked != "acorn" {
					t.Errorf("PickNext() = %q, want first name in sorted list (acorn)", picked)
				}
			},
		},
		{
			name:     "empty exclusions returns first name",
			excluded: []string{},
			wantErr:  nil,
			validatePick: func(t *testing.T, picked string) {
				if picked != "acorn" {
					t.Errorf("PickNext() = %q, want acorn", picked)
				}
			},
		},
		{
			name:     "exclude first name returns second",
			excluded: []string{"acorn"},
			wantErr:  nil,
			validatePick: func(t *testing.T, picked string) {
				if picked == "acorn" {
					t.Errorf("PickNext() returned excluded name: %q", picked)
				}
				if picked != "ash" {
					t.Errorf("PickNext() = %q, want ash", picked)
				}
			},
		},
		{
			name:     "exclude multiple names",
			excluded: []string{"acorn", "ash", "aspen"},
			wantErr:  nil,
			validatePick: func(t *testing.T, picked string) {
				for _, excluded := range []string{"acorn", "ash", "aspen"} {
					if picked == excluded {
						t.Errorf("PickNext() returned excluded name: %q", picked)
					}
				}
				if picked != "birch" {
					t.Errorf("PickNext() = %q, want birch", picked)
				}
			},
		},
		{
			name:     "case insensitive exclusion",
			excluded: []string{"ACORN", "Ash", "AsPeN"},
			wantErr:  nil,
			validatePick: func(t *testing.T, picked string) {
				for _, excluded := range []string{"acorn", "ash", "aspen"} {
					if picked == excluded {
						t.Errorf("PickNext() returned excluded name: %q", picked)
					}
				}
			},
		},
		{
			name:     "whitespace trimming in exclusions",
			excluded: []string{" acorn ", "\tash\t", "\naspen\n"},
			wantErr:  nil,
			validatePick: func(t *testing.T, picked string) {
				for _, excluded := range []string{"acorn", "ash", "aspen"} {
					if picked == excluded {
						t.Errorf("PickNext() returned excluded name: %q", picked)
					}
				}
			},
		},
		{
			name:     "empty strings in exclusions are ignored",
			excluded: []string{"", "  ", "\t", "acorn"},
			wantErr:  nil,
			validatePick: func(t *testing.T, picked string) {
				if picked == "acorn" {
					t.Errorf("PickNext() returned excluded name: %q", picked)
				}
			},
		},
		{
			name:     "all names excluded returns error",
			excluded: All(),
			wantErr:  ErrNoAvailableNames,
			validatePick: func(t *testing.T, picked string) {
				if picked != "" {
					t.Errorf("PickNext() = %q, want empty string when error returned", picked)
				}
			},
		},
		{
			name:     "duplicate exclusions handled correctly",
			excluded: []string{"acorn", "acorn", "ash", "ash"},
			wantErr:  nil,
			validatePick: func(t *testing.T, picked string) {
				if picked == "acorn" || picked == "ash" {
					t.Errorf("PickNext() returned excluded name: %q", picked)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			picked, err := PickNext(tt.excluded)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("PickNext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.validatePick != nil {
				tt.validatePick(t, picked)
			}

			// Verify picked name is in the pool (if no error)
			if err == nil && picked != "" {
				allNames := All()
				found := false
				for _, name := range allNames {
					if name == picked {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("PickNext() = %q, which is not in the fairy name pool", picked)
				}
			}
		})
	}
}

func TestAll(t *testing.T) {
	t.Parallel()

	names := All()

	if len(names) == 0 {
		t.Fatal("All() returned empty slice")
	}

	// Verify alphabetical sorting (check before modification)
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Errorf("All() not sorted: %q >= %q at positions %d, %d", names[i-1], names[i], i-1, i)
		}
	}

	// Verify it returns a copy (modifying result shouldn't affect original)
	original := names[0]
	names[0] = "modified"
	namesAgain := All()
	if namesAgain[0] != original {
		t.Errorf("All() does not return a copy; modification affected original pool")
	}

	// Verify no duplicates
	seen := make(map[string]bool)
	for _, name := range names {
		if seen[name] {
			t.Errorf("All() contains duplicate: %q", name)
		}
		seen[name] = true
	}

	// Verify no empty strings
	for i, name := range names {
		if name == "" {
			t.Errorf("All() contains empty string at position %d", i)
		}
	}
}

func TestCount(t *testing.T) {
	t.Parallel()

	count := Count()

	if count <= 0 {
		t.Errorf("Count() = %d, want positive number", count)
	}

	// Verify count matches actual pool size
	all := All()
	if count != len(all) {
		t.Errorf("Count() = %d, but All() returned %d names", count, len(all))
	}

	// Verify we have at least 50 names as per requirement
	if count < 50 {
		t.Errorf("Count() = %d, want at least 50 names", count)
	}
}

func TestFairyNamePool(t *testing.T) {
	t.Parallel()

	// Verify specific required names are present
	requiredNames := []string{
		"bramble", "fern", "moss", "thorn", "willow",
		"thistle", "hazel", "sage", "clover", "ivy",
		"wren", "ember", "cedar", "brook",
	}

	allNames := All()
	nameSet := make(map[string]bool)
	for _, name := range allNames {
		nameSet[name] = true
	}

	for _, required := range requiredNames {
		if !nameSet[required] {
			t.Errorf("Fairy name pool missing required name: %q", required)
		}
	}
}

func TestPickNextSequential(t *testing.T) {
	t.Parallel()

	// Test that we can pick all names sequentially
	excluded := []string{}
	picked := []string{}
	expectedCount := Count()

	for i := 0; i < expectedCount; i++ {
		name, err := PickNext(excluded)
		if err != nil {
			t.Fatalf("PickNext() iteration %d error = %v, want nil", i, err)
		}
		if name == "" {
			t.Fatalf("PickNext() iteration %d returned empty string", i)
		}
		picked = append(picked, name)
		excluded = append(excluded, name)
	}

	// Verify we picked exactly the right number of unique names
	if len(picked) != expectedCount {
		t.Errorf("Picked %d names, want %d", len(picked), expectedCount)
	}

	uniquePicked := make(map[string]bool)
	for _, name := range picked {
		uniquePicked[name] = true
	}
	if len(uniquePicked) != expectedCount {
		t.Errorf("Picked %d unique names, want %d", len(uniquePicked), expectedCount)
	}

	// Next pick should return error
	name, err := PickNext(excluded)
	if !errors.Is(err, ErrNoAvailableNames) {
		t.Errorf("PickNext() after exhaustion error = %v, want %v", err, ErrNoAvailableNames)
	}
	if name != "" {
		t.Errorf("PickNext() after exhaustion = %q, want empty string", name)
	}
}

func TestPickNextNonExistentExclusions(t *testing.T) {
	t.Parallel()

	// Excluding names not in the pool should not affect selection
	excluded := []string{"notaname", "fakename", "doesnotexist"}
	name, err := PickNext(excluded)
	if err != nil {
		t.Fatalf("PickNext() error = %v, want nil", err)
	}
	if name != "acorn" {
		t.Errorf("PickNext() = %q, want acorn", name)
	}
}
