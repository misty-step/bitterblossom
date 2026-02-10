package names

import (
	"fmt"
	"strconv"
	"strings"
)

// spriteNames is the canonical pool of botanical/nature sprite names.
// All entries are lowercase, single-word DNS labels.
// Unexported to prevent mutation — access via PickName, NameIndex, AllNames.
var spriteNames = [...]string{
	"bramble",
	"fern",
	"moss",
	"thistle",
	"ivy",
	"hazel",
	"rowan",
	"sage",
	"briar",
	"cedar",
	"elm",
	"juniper",
	"laurel",
	"maple",
	"oak",
	"pine",
	"reed",
	"spruce",
	"thorn",
	"willow",
	"alder",
	"ash",
	"birch",
	"clover",
	"dahlia",
	"flax",
	"gorse",
	"heath",
	"iris",
	"jade",
	"kelp",
	"larch",
	"myrtle",
	"nettle",
	"olive",
	"poplar",
	"quince",
	"rue",
	"sorrel",
	"tansy",
}

// Count returns the number of base names in the pool.
func Count() int {
	return len(spriteNames)
}

// AllNames returns a copy of the name pool (safe to modify).
func AllNames() []string {
	out := make([]string, len(spriteNames))
	copy(out, spriteNames[:])
	return out
}

// PickName returns the name at index from the sprite pool.
// Returns an error if index is negative.
//
// If index >= Count(), it wraps and appends a suffix:
//   - index 40 -> "bramble-2"
//   - index 80 -> "bramble-3"
func PickName(index int) (string, error) {
	if index < 0 {
		return "", fmt.Errorf("names: invalid index %d (must be >= 0)", index)
	}

	n := len(spriteNames)
	base := spriteNames[index%n]

	wrap := index / n
	if wrap == 0 {
		return base, nil
	}

	// wrap=1 => "-2", wrap=2 => "-3", etc.
	return base + "-" + strconv.Itoa(wrap+1), nil
}

// NameIndex returns the index of name in the sprite pool.
// Returns -1 if not present.
// Handles both base names ("bramble") and suffixed names ("bramble-2").
func NameIndex(name string) int {
	// Try direct base name match first.
	for i, candidate := range spriteNames {
		if candidate == name {
			return i
		}
	}

	// Try suffixed name: "bramble-2" -> base="bramble", suffix=2, index = (suffix-1)*Count + baseIndex.
	if idx := strings.LastIndex(name, "-"); idx > 0 {
		base := name[:idx]
		suffixStr := name[idx+1:]
		suffix, err := strconv.Atoi(suffixStr)
		if err != nil || suffix < 2 {
			return -1
		}
		for i, candidate := range spriteNames {
			if candidate == base {
				return (suffix-1)*len(spriteNames) + i
			}
		}
	}

	return -1
}
