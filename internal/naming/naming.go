package naming

import (
	"errors"
	"sort"
	"strings"
)

// ErrNoAvailableNames is returned when all fairy names are already in use.
var ErrNoAvailableNames = errors.New("naming: all fairy names are exhausted")

// fairyNames is the pool of available fairy/forest names for sprites.
var fairyNames = []string{
	"acorn",
	"ash",
	"aspen",
	"birch",
	"bramble",
	"brook",
	"cedar",
	"clover",
	"crimson",
	"daisy",
	"dawn",
	"ember",
	"fern",
	"flint",
	"frost",
	"hazel",
	"heather",
	"holly",
	"iris",
	"ivy",
	"juniper",
	"lark",
	"laurel",
	"lichen",
	"lilac",
	"linden",
	"maple",
	"meadow",
	"moss",
	"nettle",
	"oak",
	"olive",
	"petal",
	"pine",
	"poplar",
	"quill",
	"reed",
	"rowan",
	"rue",
	"rush",
	"sage",
	"sedge",
	"sky",
	"sorrel",
	"sparrow",
	"spruce",
	"starling",
	"stone",
	"thistle",
	"thorn",
	"thyme",
	"vale",
	"violet",
	"willow",
	"wren",
	"yarrow",
}

// PickNext selects the next available fairy name that is not in the excludedNames set.
// Returns ErrNoAvailableNames if all names are taken.
func PickNext(excludedNames []string) (string, error) {
	excluded := make(map[string]struct{}, len(excludedNames))
	for _, name := range excludedNames {
		normalized := strings.ToLower(strings.TrimSpace(name))
		if normalized != "" {
			excluded[normalized] = struct{}{}
		}
	}

	for _, name := range fairyNames {
		if _, taken := excluded[name]; !taken {
			return name, nil
		}
	}

	return "", ErrNoAvailableNames
}

// All returns a copy of all available fairy names, sorted alphabetically.
func All() []string {
	names := make([]string, len(fairyNames))
	copy(names, fairyNames)
	sort.Strings(names)
	return names
}

// Count returns the total number of fairy names in the pool.
func Count() int {
	return len(fairyNames)
}
