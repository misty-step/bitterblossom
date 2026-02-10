package names

import "strconv"

// SpriteNames is the canonical pool of botanical/nature sprite names.
// All entries are lowercase, single-word DNS labels.
var SpriteNames = []string{
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

func Count() int {
	return len(SpriteNames)
}

// PickName returns the name at index from SpriteNames.
//
// If index is >= Count(), it wraps and appends a suffix:
//   - index 40 -> "bramble-2"
//   - index 80 -> "bramble-3"
func PickName(index int) string {
	if index < 0 {
		return ""
	}

	n := len(SpriteNames)
	base := SpriteNames[index%n]

	wrap := index / n
	if wrap == 0 {
		return base
	}

	// wrap=1 => "-2", wrap=2 => "-3", etc.
	return base + "-" + strconv.Itoa(wrap+1)
}

// NameIndex returns the index of name in SpriteNames, or -1 if not present.
// It only matches base names (no suffix handling).
func NameIndex(name string) int {
	for i, candidate := range SpriteNames {
		if candidate == name {
			return i
		}
	}
	return -1
}
