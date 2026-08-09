package banner

import (
	"fmt"
	"regexp"
)

// nameRE governs the allowed characters for a banner name. Names are used to
// build the HTML-comment markers, so they are deliberately restricted to a
// single word of lowercase letters, digits and hyphens. Uppercase or
// space-containing names never produce a marker — a body that merely looks
// like a marker but does not match this shape is treated as ordinary text.
var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// ValidateName reports whether name is a valid banner name.
func ValidateName(name string) error {
	if nameRE.MatchString(name) {
		return nil
	}
	return fmt.Errorf("invalid banner name %q: must be 1-64 characters consisting of lowercase letters, digits, and hyphens, starting with a letter or digit", name)
}
