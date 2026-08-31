package banner

import (
	"strings"
	"testing"
)

// The TLDR banner kind: a banner named "tldr" whose region carries an
// author-written summary plus the head SHA it describes. Freshness is computed
// by the READER (by comparing changed-file sets), never enforced here — this
// package only stores and parses the banner's payload.

func TestTLDRSet(t *testing.T) {
	out, err := Apply("", TLDRName, TLDRContent("deadbeef", "ships a thing"), OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set tldr: %v", err)
	}
	if out.Action != ActionSet {
		t.Fatalf("action = %s, want set", out.Action)
	}
	// The SHA line is an HTML comment: invisible when the body renders, still
	// parseable by this package.
	want := "<!-- gh-pr-banner:tldr -->\n\n<!-- gh-pr-banner:tldr-head-sha: deadbeef -->\nships a thing\n\n<!-- /gh-pr-banner:tldr -->"
	if out.Body != want {
		t.Fatalf("body:\n%s\nwant:\n%s", out.Body, want)
	}
}

func TestTLDRContentFormatsAndParses(t *testing.T) {
	content := TLDRContent("abc123", "ships the fix.")
	sha, text, err := ParseTLDR(content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sha != "abc123" {
		t.Fatalf("sha = %q, want abc123", sha)
	}
	if text != "ships the fix." {
		t.Fatalf("text = %q, want ships the fix.", text)
	}
}

func TestTLDRContentIdempotent(t *testing.T) {
	// Re-applying TLDRContent to content that already carries the SHA line is
	// a no-op: the banner stays canonical.
	once := TLDRContent("abc123", "one-liner")
	twice := TLDRContent("abc123", once)
	if once != twice {
		t.Fatalf("not idempotent:\n%q\n%q", once, twice)
	}
}

func TestTLDRParseMultipleLinesOfText(t *testing.T) {
	content := "tldr-head-sha: abc123\nfirst line\nsecond line"
	sha, text, err := ParseTLDR(content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sha != "abc123" {
		t.Fatalf("sha = %q, want abc123", sha)
	}
	if text != "first line\nsecond line" {
		t.Fatalf("text = %q, want two lines", text)
	}
}

func TestTLDRParseMissingSHAIsError(t *testing.T) {
	if _, _, err := ParseTLDR("just some text, no sha line"); err == nil {
		t.Fatal("expected error for missing tldr-head-sha comment, got nil")
	}
}

func TestTLDRParseWrongPositionIsError(t *testing.T) {
	// The SHA comment is a managed, positional field: it must be the FIRST
	// line. Anywhere else it is prose that happens to look like one, not
	// metadata.
	if _, _, err := ParseTLDR("some text\n<!-- gh-pr-banner:tldr-head-sha: abc123 -->"); err == nil {
		t.Fatal("expected error for misplaced tldr-head-sha comment, got nil")
	}
}

func TestTLDRParseEmptySHAIsError(t *testing.T) {
	if _, _, err := ParseTLDR("<!-- gh-pr-banner:tldr-head-sha:  -->"); err == nil {
		t.Fatal("expected error for empty sha, got nil")
	}
}

func TestTLDRParseWhitespaceIsTrimmed(t *testing.T) {
	sha, text, err := ParseTLDR("<!-- gh-pr-banner:tldr-head-sha:   abc123   -->\n\n  ships a thing\n\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sha != "abc123" {
		t.Fatalf("sha = %q, want abc123", sha)
	}
	if text != "ships a thing" {
		t.Fatalf("text = %q, want ships a thing", text)
	}
}

func TestTLDRGetViaLookup(t *testing.T) {
	body := "# PR title\n\nSome prose.\n\n" +
		"<!-- gh-pr-banner:tldr -->\n\n<!-- gh-pr-banner:tldr-head-sha: deadbeef -->\nships a thing\n\n" +
		"<!-- /gh-pr-banner:tldr -->\n\nmore prose"
	ok, content, err := Lookup(body, TLDRName)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if !ok {
		t.Fatal("tldr banner not found")
	}
	sha, text, err := ParseTLDR(content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sha != "deadbeef" {
		t.Fatalf("sha = %q, want deadbeef", sha)
	}
	if text != "ships a thing" {
		t.Fatalf("text = %q, want ships a thing", text)
	}
	if strings.Contains(text, "tldr-head-sha:") {
		t.Fatalf("SHA line leaked into the reader-visible text: %q", text)
	}
}

func TestTLDRParseLegacyPlainSHAStillReads(t *testing.T) {
	// Bodies written before the SHA moved into an HTML comment carry a plain
	// "tldr-head-sha:" first line. That previous canonical form is
	// unambiguous, so reading it keeps working; only writing uses the
	// comment form.
	sha, text, err := ParseTLDR("tldr-head-sha: deadbeef\nships a thing")
	if err != nil {
		t.Fatalf("parse legacy form: %v", err)
	}
	if sha != "deadbeef" || text != "ships a thing" {
		t.Fatalf("sha = %q, text = %q; want deadbeef / ships a thing", sha, text)
	}
}

func TestTLDRContentHidesSHAFromRenderedView(t *testing.T) {
	// The stored SHA must live inside an HTML comment so it is invisible when
	// the body renders, while ParseTLDR still returns it.
	content := TLDRContent("abc123", "ships the fix.")
	if !strings.HasPrefix(content, "<!-- ") || !strings.Contains(content, "-->") {
		t.Fatalf("SHA is not stored in an HTML comment: %q", content)
	}
	parsed, _, err := ParseTLDR(content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed != "abc123" {
		t.Fatalf("sha = %q, want abc123", parsed)
	}
}

func TestTLDRNameIsValid(t *testing.T) {
	if err := ValidateName(TLDRName); err != nil {
		t.Fatalf("tldr should be a valid banner name: %v", err)
	}
}

func TestTLDRSHACommentIsNotAManagedMarker(t *testing.T) {
	// The SHA comment is shaped like the region markers but "tldr-head-sha"
	// is not a valid banner name, so the marker parser must never treat it as
	// a managed region: applying another banner to a body that carries a tldr
	// must succeed and leave the comment untouched.
	body := "<!-- gh-pr-banner:tldr -->\n\n<!-- gh-pr-banner:tldr-head-sha: deadbeef -->\nsummary\n\n<!-- /gh-pr-banner:tldr -->"
	out, err := Apply(body, "do-not-merge", "RED", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("apply alongside a tldr banner: %v", err)
	}
	if !strings.Contains(out.Body, "<!-- gh-pr-banner:tldr-head-sha: deadbeef -->") {
		t.Fatalf("SHA comment was not preserved:\n%s", out.Body)
	}
	// And the tldr still parses out of the spliced body.
	ok, content, err := Lookup(out.Body, TLDRName)
	if err != nil || !ok {
		t.Fatalf("lookup after splice: ok=%v err=%v", ok, err)
	}
	if sha, _, err := ParseTLDR(content); err != nil || sha != "deadbeef" {
		t.Fatalf("sha = %q, err = %v; want deadbeef / nil", sha, err)
	}
}

func TestTLDRParseSHACommentWithoutCloseBraceIsNotSHA(t *testing.T) {
	// A first line that opens like the SHA comment but never closes it is
	// prose, not metadata.
	if _, _, err := ParseTLDR("<!-- gh-pr-banner:tldr-head-sha: deadbeef\nsummary"); err == nil {
		t.Fatal("expected error for unterminated SHA comment, got nil")
	}
}
