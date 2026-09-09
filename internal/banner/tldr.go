// TLDR banner kind: an author-written summary plus the SHA it describes.
//
// A banner named TLDRName ("tldr") whose region holds:
//
//	<!-- gh-pr-banner:tldr tldr-head-sha: <sha> -->
//	> **TLDR**
//	<one or two author-written sentences>
//
//	<!-- /gh-pr-banner:tldr -->
//
// The SHA travels IN the opening region comment, so it is invisible when the
// body renders while staying parseable by this package. The "> **TLDR**"
// label is managed payload: the writer emits it, the parser strips it. The
// SHA is stored, never enforced — freshness is computed by the READER (which
// compares changed-file sets and renders the banner marked stale when they
// differ; a head that moved with an identical file set stays fresh). Nothing
// here ever generates the summary from a diff: the whole value of a TLDR is
// that a human wrote it.
package banner

import (
	"fmt"
	"strings"
)

// TLDRName is the banner name for the TLDR kind. It is a fixed, reserved name:
// readers look for this name specifically, not for a name shape.
const TLDRName = "tldr"

// TLDRLabel is the fixed visible label line rendered immediately under the
// opener. It is managed payload: the writer emits it, the parser strips it,
// and prose that begins with it is never reader content.
const TLDRLabel = "> **TLDR**"

// TLDRSHAPrefix is the opener-carried attribute carrying the head SHA the
// banner describes.
const TLDRSHAPrefix = "tldr-head-sha:"

// tldrSHACommentOpen/close fence the intermediate HTML-comment form — a
// standalone comment line inside the region, with no label — written by an
// older version. It remains readable.
const (
	tldrSHACommentOpen  = "<!-- gh-pr-banner:tldr-head-sha:"
	tldrSHACommentClose = "-->"
)

// tldrSHALegacyPrefix is the earliest canonical first line, where the SHA sat
// in plain text. Old bodies in this shape remain readable; new writes always
// carry the SHA in the opener.
const tldrSHALegacyPrefix = "tldr-head-sha:"

// TLDROpener renders the opening marker line for a TLDR describing sha:
// the marker comment with the SHA carried inside it as an attribute.
func TLDROpener(sha string) string {
	return openPrefix + TLDRName + " " + TLDRSHAPrefix + " " + strings.TrimSpace(sha) + markerSfx
}

// tldrBody renders the banner content, excluding the opener: the label line,
// then the author's summary. text with no summary yields just the label.
func tldrBody(text string) string {
	return TLDRLabel + "\n" + strings.TrimSpace(text)
}

// TLDRApply splices a TLDR banner describing sha, with the author-written
// summary, into body. The SHA lives in the opening region comment; the label
// is part of the payload. Re-stamping the same summary + SHA is idempotent
// end-to-end (Action Unchanged, no network write); re-stamping replaces in
// place, never appends.
func TLDRApply(body, sha, text string, op Operation, at Placement) (Outcome, error) {
	return ApplyOpener(body, TLDRName, TLDROpener(sha), tldrBody(text), op, at)
}

// ParseTLDR extracts the SHA and the author-written summary from TLDR banner
// content as returned by LookupOpener(body, TLDRName) — opener being the
// region's opening marker line and content everything after it. It fails
// loudly rather than guess when the payload is not in the managed shape: the
// first content line must carry the SHA in some recognized form.
//
// Three reader-visible shapes of the SHA remain readable, so an
// already-stamped PR never becomes unparseable:
//
//   - opener-carried: "<!-- gh-pr-banner:tldr tldr-head-sha: <sha> -->"
//   - standalone comment (intermediate): "<!-- gh-pr-banner:tldr-head-sha: <sha> -->"
//   - plain text (legacy): "tldr-head-sha: <sha>"
func ParseTLDR(opener, content string) (sha, text string, err error) {
	sha, err = parseOpenerSHA(opener)
	if err != nil {
		// No SHA in the opener: fall back to reading the payload's first line
		// in one of the older shapes.
		return parseLegacyTLDR(content)
	}
	return sha, stripLabel(content), nil
}

// parseOpenerSHA extracts the SHA attribute from a TLDR opener line, or an
// error when the opener carries none.
func parseOpenerSHA(opener string) (string, error) {
	// Trim the marker scaffolding: "<!-- gh-pr-banner:tldr" ... " -->".
	if !strings.HasPrefix(opener, openPrefix+TLDRName) || !strings.HasSuffix(opener, markerSfx) {
		return "", fmt.Errorf("not a tldr opener")
	}
	// The opener body after the name is " tldr-head-sha: <sha>" (leading
	// space always present) — or empty for the bare-name form.
	attr := strings.TrimSuffix(strings.TrimPrefix(opener, openPrefix+TLDRName), markerSfx)
	rest, found := strings.CutPrefix(attr, " "+TLDRSHAPrefix+" ")
	if !found {
		return "", fmt.Errorf("tldr opener carries no %s attribute", TLDRSHAPrefix)
	}
	if sha := strings.TrimSpace(rest); sha != "" {
		return sha, nil
	}
	return "", fmt.Errorf("tldr opener carries an empty %s attribute", TLDRSHAPrefix)
}

// parseLegacyTLDR reads the SHA from the payload's first line when the opener
// carries no SHA, covering both older shapes.
func parseLegacyTLDR(content string) (sha, text string, err error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", "", fmt.Errorf("tldr banner is empty")
	}
	lines := strings.SplitN(content, "\n", 2)
	first := strings.TrimSpace(lines[0])
	s, ok := parseSHALineLegacy(first)
	if !ok {
		return "", "", fmt.Errorf("tldr banner is not in the managed shape: no %s in the opener and first line must carry it, found %q — refusing to guess", TLDRSHAPrefix, first)
	}
	rest := ""
	if len(lines) == 2 {
		rest = strings.TrimSpace(lines[1])
	}
	return s, stripLabel(rest), nil
}

// stripLabel removes the managed TLDR label line from the start of the
// summary, and trims the remainder — the label is never reader-visible text.
func stripLabel(text string) string {
	text = strings.TrimSpace(text)
	if text == TLDRLabel {
		return ""
	}
	if rest, found := strings.CutPrefix(text, TLDRLabel+"\n"); found {
		return strings.TrimSpace(rest)
	}
	return text
}

// parseSHALineLegacy extracts the SHA from one line in either older form:
// standalone HTML comment or plain text. (The opener-carried form is handled
// by parseOpenerSHA.)
func parseSHALineLegacy(line string) (string, bool) {
	if rest, found := strings.CutPrefix(line, tldrSHACommentOpen); found {
		if !strings.HasSuffix(rest, " "+tldrSHACommentClose) {
			return "", false
		}
		if sha := strings.TrimSpace(strings.TrimSuffix(rest, " "+tldrSHACommentClose)); sha != "" {
			return sha, true
		}
		return "", false
	}
	if rest, found := strings.CutPrefix(line, tldrSHALegacyPrefix); found {
		if sha := strings.TrimSpace(rest); sha != "" {
			return sha, true
		}
		return "", false
	}
	return "", false
}
