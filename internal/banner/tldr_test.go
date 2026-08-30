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
	want := "<!-- gh-pr-banner:tldr -->\n\ntldr-head-sha: deadbeef\nships a thing\n\n<!-- /gh-pr-banner:tldr -->"
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
		t.Fatal("expected error for missing tldr-head-sha line, got nil")
	}
}

func TestTLDRParseWrongPositionIsError(t *testing.T) {
	// The SHA line is a managed, positional field: it must be the FIRST line.
	// Anywhere else it is prose that happens to look like one, not metadata.
	if _, _, err := ParseTLDR("some text\ntldr-head-sha: abc123"); err == nil {
		t.Fatal("expected error for misplaced tldr-head-sha line, got nil")
	}
}

func TestTLDRParseEmptySHAIsError(t *testing.T) {
	if _, _, err := ParseTLDR("tldr-head-sha:"); err == nil {
		t.Fatal("expected error for empty sha, got nil")
	}
}

func TestTLDRParseWhitespaceIsTrimmed(t *testing.T) {
	sha, text, err := ParseTLDR("tldr-head-sha:   abc123   \n\n  ships a thing\n\n")
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
		"<!-- gh-pr-banner:tldr -->\n\ntldr-head-sha: deadbeef\nships a thing\n\n" +
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

func TestTLDRNameIsValid(t *testing.T) {
	if err := ValidateName(TLDRName); err != nil {
		t.Fatalf("tldr should be a valid banner name: %v", err)
	}
}
