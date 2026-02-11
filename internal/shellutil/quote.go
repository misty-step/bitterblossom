// Package shellutil provides safe POSIX shell string construction primitives
// for building remote commands sent to sprites.
package shellutil

import "strings"

// Quote wraps a value in single quotes, escaping any embedded single quotes
// for safe use in shell commands (POSIX sh-compatible).
func Quote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
