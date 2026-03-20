package config

import (
	"fmt"
	"regexp"
)

var workspaceRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// ValidateWorkspace checks that a workspace name is safe.
func ValidateWorkspace(name string) error {
	if !workspaceRe.MatchString(name) {
		return fmt.Errorf("invalid workspace name %q: must be 1-64 alphanumeric characters, hyphens, or underscores", name)
	}
	return nil
}
