package lifecycle

import (
	"fmt"
	"regexp"
	"strings"
)

var validSpriteName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// ValidateSpriteName validates sprite names used across lifecycle commands.
func ValidateSpriteName(name string) error {
	trimmed := strings.TrimSpace(name)
	if !validSpriteName.MatchString(trimmed) {
		return fmt.Errorf("invalid sprite name %q: use lowercase alphanumeric + hyphens", name)
	}
	return nil
}
