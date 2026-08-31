package banner

import (
	"strings"
	"testing"
)

// The TLDR banner kind: a banner named "tldr" whose region carries an
// author-written summary plus the head SHA it describes. The SHA travels in
// the OPENING region marker, so it is invisible when the body renders while
// staying machine-readable. Freshness is computed by the READER (by comparing
// changed-file sets), never enforced here — this package only stores and
// parses the banner's payload.

// A neutral, invented example: the exact bytes a stamp produces.
const (
	exampleSHA  = "eecb2135a5a1b0678d032add464cba110e0d8f90"
	exampleText = "The retry loop now backs off exponentially instead of\nhammering the staging endpoint; the flaky tests that timed out under\nload pass consistently."
)

var wantTLDRBlock = "<!-- gh-pr-banner:tldr tldr-head-sha: " + exampleSHA + " -->\n" +
	"> **TLDR**\n" +
	exampleText + "\n" +
	"\n" +
	"<!-- /gh-pr-banner:tldr -->"

// readTLDR is the test helper for the reader path: LookupOpener + ParseTLDR.
func readTLDR(t *testing.T, body string) (sha, text string) {
	t.Helper()
	ok, opener, content, err := LookupOpener(body, TLDRName)
	if err != nil || !ok {
		t.Fatalf("lookup: ok=%v err=%v", ok, err)
	}
	sha, text, err = ParseTLDR(opener, content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return sha, text
}

func TestTLDRSet(t *testing.T) {
	out, err := TLDRApply("", exampleSHA, exampleText, OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set tldr: %v", err)
	}
	if out.Action != ActionSet {
		t.Fatalf("action = %s, want set", out.Action)
	}
	if out.Body != wantTLDRBlock {
		t.Fatalf("body:\n%s\nwant:\n%s", out.Body, wantTLDRBlock)
	}
}

func TestTLDRSetAndReadBack(t *testing.T) {
	// Round trip: stamp, parse, get the same SHA and text back.
	out, err := TLDRApply("", exampleSHA, exampleText, OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	sha, text := readTLDR(t, out.Body)
	if sha != exampleSHA || text != exampleText {
		t.Fatalf("round trip: sha = %q, text = %q", sha, text)
	}
}

func TestTLDRLabelIsManagedPayload(t *testing.T) {
	// The opener is followed by the label, then the prose; the label never
	// leaks into the reader-visible text.
	out, err := TLDRApply("", "abc123", "one line\nsecond line", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	_, text := readTLDR(t, out.Body)
	if text != "one line\nsecond line" {
		t.Fatalf("text = %q, want one line\\nsecond line", text)
	}
	if !strings.HasPrefix(out.Body, "<!-- gh-pr-banner:tldr tldr-head-sha: abc123 -->\n> **TLDR**\n") {
		t.Fatalf("label not immediately under the opener:\n%s", out.Body)
	}
}

func TestTLDRContentIdempotent(t *testing.T) {
	// Re-stamping the same summary + SHA is a no-op: unchanged, no append.
	once, err := TLDRApply("", "abc123", "one-liner", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("first stamp: %v", err)
	}
	twice, err := TLDRApply(once.Body, "abc123", "one-liner", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("second stamp: %v", err)
	}
	if twice.Action != ActionUnchanged {
		t.Fatalf("action = %s, want unchanged", twice.Action)
	}
	if twice.Body != once.Body {
		t.Fatalf("body changed:\n%q\n%q", once.Body, twice.Body)
	}
}

func TestTLDRParseLegacyPlainSHAStillReads(t *testing.T) {
	// Bodies written before the SHA moved into the opener carry a plain
	// "tldr-head-sha:" first line and no label; the region opener is the bare
	// name. Reading that shape keeps working; only writing uses the
	// opener-carried form.
	body := "<!-- gh-pr-banner:tldr -->\n\ntldr-head-sha: deadbeef\nships a thing\n\n<!-- /gh-pr-banner:tldr -->"
	sha, text := readTLDR(t, body)
	if sha != "deadbeef" || text != "ships a thing" {
		t.Fatalf("sha = %q, text = %q; want deadbeef / ships a thing", sha, text)
	}
}

func TestTLDRParseLegacyCommentSHAStillReads(t *testing.T) {
	// An intermediate form stored the SHA in a standalone HTML comment inside
	// the region, with no label. Reading it keeps working too.
	body := "<!-- gh-pr-banner:tldr -->\n\n<!-- gh-pr-banner:tldr-head-sha: deadbeef -->\nships a thing\n\n<!-- /gh-pr-banner:tldr -->"
	sha, text := readTLDR(t, body)
	if sha != "deadbeef" || text != "ships a thing" {
		t.Fatalf("sha = %q, text = %q; want deadbeef / ships a thing", sha, text)
	}
}

func TestTLDRParseSHACommentWithoutCloseBraceIsNotSHA(t *testing.T) {
	// A payload first line that opens like a SHA comment but never closes it is
	// prose, not metadata: parse must refuse.
	body := "<!-- gh-pr-banner:tldr -->\n\n<!-- gh-pr-banner:tldr-head-sha: deadbeef\nsummary\n\n<!-- /gh-pr-banner:tldr -->"
	ok, opener, content, err := LookupOpener(body, TLDRName)
	if err != nil || !ok {
		t.Fatalf("lookup: ok=%v err=%v", ok, err)
	}
	if _, _, err := ParseTLDR(opener, content); err == nil {
		t.Fatal("expected error for unterminated SHA comment, got nil")
	}
}

func TestTLDRParseMissingSHAIsError(t *testing.T) {
	// No SHA in the opener and the payload is prose only: a real error.
	body := "<!-- gh-pr-banner:tldr -->\n\n> **TLDR**\njust some text\n\n<!-- /gh-pr-banner:tldr -->"
	ok, opener, content, err := LookupOpener(body, TLDRName)
	if err != nil || !ok {
		t.Fatalf("lookup: ok=%v err=%v", ok, err)
	}
	if _, _, err := ParseTLDR(opener, content); err == nil {
		t.Fatal("expected error for missing tldr-head-sha, got nil")
	}
}

func TestTLDRParseEmptyInOpenerIsError(t *testing.T) {
	// An opener carrying " tldr-head-sha: (nothing)" fails to parse.
	body := "<!-- gh-pr-banner:tldr tldr-head-sha:  -->\n\nships a thing\n\n<!-- /gh-pr-banner:tldr -->"
	ok, opener, content, err := LookupOpener(body, TLDRName)
	if err != nil || !ok {
		t.Fatalf("lookup: ok=%v err=%v", ok, err)
	}
	if _, _, err := ParseTLDR(opener, content); err == nil {
		t.Fatal("expected error for an empty opener-carried sha, got nil")
	}
}

func TestTLDRParseWhitespaceIsTrimmed(t *testing.T) {
	sha, text := readTLDR(t, "<!-- gh-pr-banner:tldr tldr-head-sha: abc123 -->\n\n  ships a thing\n\n<!-- /gh-pr-banner:tldr -->")
	if sha != "abc123" {
		t.Fatalf("sha = %q, want abc123", sha)
	}
	if text != "ships a thing" {
		t.Fatalf("text = %q, want ships a thing", text)
	}
}

func TestTLDRGetViaLookup(t *testing.T) {
	body := "# PR title\n\nSome prose.\n\n" + wantTLDRBlock + "\n\nmore prose"
	sha, text := readTLDR(t, body)
	if sha != exampleSHA {
		t.Fatalf("sha = %q, want %s", sha, exampleSHA)
	}
	if text != exampleText {
		t.Fatalf("text = %q, want the example prose", text)
	}
	if strings.Contains(text, "tldr-head-sha:") || strings.Contains(text, "> **TLDR**") {
		t.Fatalf("managed lines leaked into the reader-visible text: %q", text)
	}
}

func TestTLDRRoundTripThroughSplice(t *testing.T) {
	// Stamp, parse, get the same SHA and text back — through the real splice
	// path, with the closing delimiter written by ApplyOpener.
	out, err := TLDRApply("", exampleSHA, exampleText, OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(out.Body, "<!-- /gh-pr-banner:tldr -->") {
		t.Fatalf("closing delimiter missing:\n%s", out.Body)
	}
	sha, text := readTLDR(t, out.Body)
	if sha != exampleSHA || text != exampleText {
		t.Fatalf("round trip mismatch: sha = %q, text = %q", sha, text)
	}
	// Re-stamping the identical summary + SHA is a no-op: idempotent, no
	// append.
	again, err := TLDRApply(out.Body, exampleSHA, exampleText, OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if again.Action != ActionUnchanged {
		t.Fatalf("re-stamp action = %s, want unchanged", again.Action)
	}
}

func TestTLDRRestampReplacesExistingRegionInPlace(t *testing.T) {
	// Re-stamping a DIFFERENT summary on an existing region replaces, never
	// appends: /pr depends on re-stamp idempotency.
	body := "pre\n\n" + wantTLDRBlock + "\n\npost"
	out, err := TLDRApply(body, exampleSHA, "second summary", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("re-stamp: %v", err)
	}
	if out.Action != ActionUpdated {
		t.Fatalf("action = %s, want updated", out.Action)
	}
	_, text := readTLDR(t, out.Body)
	if text != "second summary" {
		t.Fatalf("text = %q, want second summary", text)
	}
	if strings.Count(out.Body, exampleText) != 0 || strings.Count(out.Body, "second summary") != 1 {
		t.Fatalf("region was not replaced in place:\n%s", out.Body)
	}
	if !strings.HasPrefix(out.Body, "pre") || !strings.HasSuffix(out.Body, "post") {
		t.Fatalf("surrounding prose was lost:\n%s", out.Body)
	}
}

func TestTLDRContentEmptyText(t *testing.T) {
	// A TLDR whose summary is empty still stamps the managed shape.
	out, err := TLDRApply("", "abc123", "", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	want := "<!-- gh-pr-banner:tldr tldr-head-sha: abc123 -->\n> **TLDR**\n\n<!-- /gh-pr-banner:tldr -->"
	if out.Body != want {
		t.Fatalf("body:\n%s\nwant:\n%s", out.Body, want)
	}
	sha, text := readTLDR(t, out.Body)
	if sha != "abc123" || text != "" {
		t.Fatalf("sha = %q, text = %q; want abc123 / empty", sha, text)
	}
}

func TestTLDRSHACommentIsNotAManagedMarker(t *testing.T) {
	// Prose that happens to carry tldr-head-sha-shaped lines is not a managed
	// region: the marker parser must never treat it as one, and splicing
	// alongside a stamped tldr banner preserves it.
	legacySHAComment := "<!-- gh-pr-banner:tldr-head-sha: deadbeef -->"
	body := wantTLDRBlock + "\n\n" + legacySHAComment + "\n\ntldr-head-sha: decoy in prose"
	out, err := Apply(body, "do-not-merge", "RED", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("apply alongside a tldr banner: %v", err)
	}
	if !strings.Contains(out.Body, legacySHAComment) || !strings.Contains(out.Body, "tldr-head-sha: decoy in prose") {
		t.Fatalf("decoy lines were not preserved:\n%s", out.Body)
	}
	sha, _ := readTLDR(t, out.Body)
	if sha != exampleSHA {
		t.Fatalf("sha = %q, want %s", sha, exampleSHA)
	}
}

func TestTLDRLegacyBodyStampsUpToNewFormat(t *testing.T) {
	// A body stamped in the legacy plain-text shape (no opener SHA, no label)
	// re-stamps into the canonical shape in place — an already-stamped PR
	// does not become unparseable or duplicated when the new version runs
	// against it.
	legacyRegion := "<!-- gh-pr-banner:tldr -->\n\ntldr-head-sha: abc123\nships a thing\n\n<!-- /gh-pr-banner:tldr -->"
	out, err := TLDRApply(legacyRegion, "abc123", "ships a thing", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("re-stamp legacy body: %v", err)
	}
	if out.Action != ActionUpdated {
		t.Fatalf("action = %s, want updated", out.Action)
	}
	want := "<!-- gh-pr-banner:tldr tldr-head-sha: abc123 -->\n> **TLDR**\nships a thing\n\n<!-- /gh-pr-banner:tldr -->"
	if out.Body != want {
		t.Fatalf("legacy body was not normalized to the canonical shape:\n%s\nwant:\n%s", out.Body, want)
	}
}

func TestTLDRNestedBannerInsideOtherBannerStaysValid(t *testing.T) {
	// A tldr banner with an opener-carried SHA nests correctly inside another
	// banner region, as any properly nested pair must.
	body := "<!-- gh-pr-banner:outer -->\n\n" + wantTLDRBlock + "\n\n<!-- /gh-pr-banner:outer -->"
	names, err := List(body)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 2 || names[0] != "outer" || names[1] != TLDRName {
		t.Fatalf("names = %v, want [outer tldr]", names)
	}
	sha, _ := readTLDR(t, body)
	if sha != exampleSHA {
		t.Fatalf("sha = %q, want %s", sha, exampleSHA)
	}
}

func TestTLDRSetPreservesOtherBanners(t *testing.T) {
	other := "<!-- gh-pr-banner:do-not-merge -->\n\nRED\n\n<!-- /gh-pr-banner:do-not-merge -->"
	out, err := TLDRApply("pre\n\n"+other+"\n\npost", exampleSHA, exampleText, OpSet, PlacementBottom)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(out.Body, other) {
		t.Fatalf("existing banner was not preserved:\n%s", out.Body)
	}
	if !strings.HasPrefix(out.Body, "pre") {
		t.Fatalf("leading prose was lost:\n%s", out.Body)
	}
}