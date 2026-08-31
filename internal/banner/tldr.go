// TLDR banner kind: an author-written summary plus the SHA it describes.
//
// A banner named TLDRName ("tldr") whose region holds two things:
//
//	<!-- gh-pr-banner:tldr-head-sha: <sha> -->
//	<one or two author-written sentences>
//
// The SHA is stored inside an HTML comment so it is invisible when the body
// renders, while staying parseable by this package. It is stored, never
// enforced — freshness is computed by the READER (which compares changed-file
// sets and renders the banner marked stale when they differ; a head that moved
// with an identical file set stays fresh). Nothing here ever generates the
// summary from a diff: the whole value of a TLDR is that a human wrote it.
package banner

import (
	"fmt"
	"strings"
)

// TLDRName is the banner name for the TLDR kind. It is a fixed, reserved name:
// readers look for this name specifically, not for a name shape.
const TLDRName = "tldr"

// tldrSHAOpen/close fence the HTML comment carrying the SHA the banner
// describes. The comment must be the first line of the banner region:
// everything after it is the summary itself, which may legitimately say
// anything — including a line that happens to contain this prefix.
//
// The prefix is deliberately distinct from the region markers
// (gh-pr-banner:NAME): "tldr-head-sha" is not a valid banner name, so
// parseAndValidate never mistakes the SHA comment for a managed region.
const (
	tldrSHAOpen  = "<!-- gh-pr-banner:tldr-head-sha:"
	tldrSHAClose = "-->"
)

// tldrSHALegacyPrefix is the previous canonical first line, where the SHA sat
// in plain text. Old bodies in this shape remain readable; new writes always
// use the HTML-comment form.
const tldrSHALegacyPrefix = "tldr-head-sha:"

// TLDRContent renders the canonical banner content for a TLDR describing sha.
// The result is stable: re-wrapping content that already carries the SHA
// comment changes nothing, so re-stamping the same summary + SHA is idempotent
// end-to-end (Apply reports Unchanged and makes no network write).
//
// text is the author's summary; any sha-carrying first line in it is dropped
// so the canonical form has exactly one.
func TLDRContent(sha, text string) string {
	sha = strings.TrimSpace(sha)
	text = strings.TrimSpace(text)
	if _, _, ok := splitSHALine(text); ok {
		// Drop a legacy or comment-form SHA first line; the canonical form
		// re-adds exactly one.
		if lines := strings.SplitN(text, "\n", 2); len(lines) == 2 {
			text = strings.TrimSpace(lines[1])
		} else {
			text = ""
		}
	}
	comment := tldrSHAOpen + " " + sha + " " + tldrSHAClose
	if text == "" {
		return comment
	}
	return comment + "\n" + text
}

// ParseTLDR extracts the SHA and the author-written summary from TLDR banner
// content, as returned by Lookup(body, TLDRName). It fails loudly rather than
// guess when the payload is not in the managed shape: the first line must be
// the SHA comment (or the legacy plain-text form) and must carry a non-empty
// SHA.
func ParseTLDR(content string) (sha, text string, err error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", "", fmt.Errorf("tldr banner is empty")
	}
	lines := strings.SplitN(content, "\n", 2)
	first := strings.TrimSpace(lines[0])
	s, ok := parseSHALine(first)
	if !ok {
		return "", "", fmt.Errorf("tldr banner is not in the managed shape: first line must be the %q…%s comment, found %q — refusing to guess", tldrSHAOpen, tldrSHAClose, first)
	}
	if len(lines) == 2 {
		text = strings.TrimSpace(lines[1])
	}
	return s, text, nil
}

// splitSHALine splits the first line of text into (sha, rest, true) when it is
// a recognized SHA-carrying line, and ("", text, false) when it is not.
func splitSHALine(text string) (sha, rest string, ok bool) {
	text = strings.TrimSpace(text)
	lines := strings.SplitN(text, "\n", 2)
	if s, isSHA := parseSHALine(strings.TrimSpace(lines[0])); isSHA {
		if len(lines) == 2 {
			return s, strings.TrimSpace(lines[1]), true
		}
		return s, "", true
	}
	return "", text, false
}

// parseSHALine extracts the SHA from one line in either the current
// HTML-comment form or the legacy plain-text form.
func parseSHALine(line string) (string, bool) {
	if strings.HasPrefix(line, tldrSHAOpen) && strings.HasSuffix(line, tldrSHAClose) {
		sha := strings.TrimSpace(line[len(tldrSHAOpen) : len(line)-len(tldrSHAClose)])
		if sha != "" {
			return sha, true
		}
		return "", false
	}
	if strings.HasPrefix(line, tldrSHALegacyPrefix) {
		sha := strings.TrimSpace(strings.TrimPrefix(line, tldrSHALegacyPrefix))
		if sha != "" {
			return sha, true
		}
		return "", false
	}
	return "", false
}
