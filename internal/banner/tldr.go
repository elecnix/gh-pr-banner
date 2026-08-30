// TLDR banner kind: an author-written summary plus the SHA it describes.
//
// A banner named TLDRName ("tldr") whose region holds two things:
//
//	tldr-head-sha: <sha>
//	<one or two author-written sentences>
//
// The SHA is stored, never enforced — freshness is computed by the READER
// (which compares changed-file sets and renders the banner marked stale when
// they differ; a head that moved with an identical file set stays fresh).
// Nothing here ever generates the summary from a diff: the whole value of a
// TLDR is that a human wrote it.
package banner

import (
	"fmt"
	"strings"
)

// TLDRName is the banner name for the TLDR kind. It is a fixed, reserved name:
// readers look for this name specifically, not for a name shape.
const TLDRName = "tldr"

// tldrSHAPrefix marks the line carrying the SHA the banner describes. It must
// be the first line of the banner region: everything after it is the summary
// itself, which may legitimately say anything — including a line that happens
// to contain this prefix.
const tldrSHAPrefix = "tldr-head-sha:"

// TLDRContent renders the canonical banner content for a TLDR describing
// sha. The result is stable: re-wrapping content that already carries the SHA
// line changes nothing, so re-stamping the same summary + SHA is idempotent
// end-to-end (Apply reports Unchanged and makes no network write).
//
// text is the author's summary; any sha-carrying first line in it is dropped
// so the canonical form has exactly one.
func TLDRContent(sha, text string) string {
	sha = strings.TrimSpace(sha)
	text = strings.TrimSpace(text)
	if lines := strings.SplitN(text, "\n", 2); len(lines) == 2 && strings.HasPrefix(lines[0], tldrSHAPrefix) {
		text = strings.TrimSpace(lines[1])
	}
	if text == "" {
		return tldrSHAPrefix + " " + sha
	}
	return tldrSHAPrefix + " " + sha + "\n" + text
}

// ParseTLDR extracts the SHA and the author-written summary from TLDR banner
// content, as returned by Lookup(body, TLDRName). It fails loudly rather than
// guess when the payload is not in the managed shape: the SHA line must be the
// first line and must carry a non-empty SHA.
func ParseTLDR(content string) (sha, text string, err error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", "", fmt.Errorf("tldr banner is empty")
	}
	lines := strings.SplitN(content, "\n", 2)
	first := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(first, tldrSHAPrefix) {
		return "", "", fmt.Errorf("tldr banner is not in the managed shape: first line must be %q, found %q — refusing to guess", tldrSHAPrefix, first)
	}
	sha = strings.TrimSpace(strings.TrimPrefix(first, tldrSHAPrefix))
	if sha == "" {
		return "", "", fmt.Errorf("tldr banner carries an empty %q value", tldrSHAPrefix)
	}
	if len(lines) == 2 {
		text = strings.TrimSpace(lines[1])
	}
	return sha, text, nil
}
