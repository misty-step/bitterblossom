package lib

import "regexp"

var spriteNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var repoPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+/[a-zA-Z0-9._-]+$`)

func ValidateSpriteName(name string) error {
	if !spriteNamePattern.MatchString(name) {
		return &ValidationError{Field: "sprite", Message: "use lowercase alphanumeric plus hyphens"}
	}
	return nil
}

func ValidateRepoRef(repo string) error {
	if repoPattern.MatchString(repo) {
		return nil
	}
	if len(repo) >= len("https://") && repo[:len("https://")] == "https://" {
		return nil
	}
	return &ValidationError{Field: "repo", Message: "use org/repo or full https:// URL"}
}
