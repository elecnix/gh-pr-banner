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

// wantTLDRBlock is written out line by line rather than derived from
// exampleText, so the expected rendering is stated independently of the code
// that produces it.
var wantTLDRBlock = "<!-- gh-pr-banner:tldr tldr-head-sha: " + exampleSHA + " -->\n" +
	"> **TLDR**\n" +
	"> The retry loop now backs off exponentially instead of\n" +
	"> hammering the staging endpoint; the flaky tests that timed out under\n" +
	"> load pass consistently.\n" +
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
	if !strings.HasPrefix(out.Body, "<!-- gh-pr-banner:tldr tldr-head-sha: abc123 -->\n> **TLDR**\n> one line\n> second line\n") {
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
	want := "<!-- gh-pr-banner:tldr tldr-head-sha: abc123 -->\n> **TLDR**\n> ships a thing\n\n<!-- /gh-pr-banner:tldr -->"
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

func TestTLDRRendersSummaryAsBlockquote(t *testing.T) {
	// The whole TLDR renders as one blockquote: the label and every summary
	// line carry the "> " prefix, and a blank line separates the quote from
	// the closing marker. This is the shape a reader sees on the PR page.
	out, err := TLDRApply("", "1a2b3c4", "fixes the flaky retry loop", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	want := "<!-- gh-pr-banner:tldr tldr-head-sha: 1a2b3c4 -->\n" +
		"> **TLDR**\n" +
		"> fixes the flaky retry loop\n" +
		"\n" +
		"<!-- /gh-pr-banner:tldr -->"
	if out.Body != want {
		t.Fatalf("body:\n%s\nwant:\n%s", out.Body, want)
	}
	// The quote prefix is presentation: the reader gets the author's text back
	// exactly as written.
	_, text := readTLDR(t, out.Body)
	if text != "fixes the flaky retry loop" {
		t.Fatalf("text = %q, want the unquoted summary", text)
	}
}

func TestTLDRBlankSummaryLineKeepsOneBlockquote(t *testing.T) {
	// A blank line inside the summary must stay inside the quote, or markdown
	// would end the blockquote and render the rest as plain prose. It comes
	// back as a blank line, not as a ">".
	out, err := TLDRApply("", "abc123", "first para\n\nsecond para", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	want := "<!-- gh-pr-banner:tldr tldr-head-sha: abc123 -->\n" +
		"> **TLDR**\n" +
		"> first para\n" +
		">\n" +
		"> second para\n" +
		"\n" +
		"<!-- /gh-pr-banner:tldr -->"
	if out.Body != want {
		t.Fatalf("body:\n%s\nwant:\n%s", out.Body, want)
	}
	_, text := readTLDR(t, out.Body)
	if text != "first para\n\nsecond para" {
		t.Fatalf("text = %q, want the blank line back", text)
	}
}

func TestTLDRUnquotedSummaryStillReadsBack(t *testing.T) {
	// Bodies stamped before the summary was quoted have bare prose under the
	// label. They must still read back, unchanged.
	body := "<!-- gh-pr-banner:tldr tldr-head-sha: abc123 -->\n> **TLDR**\nships a thing\n\n<!-- /gh-pr-banner:tldr -->"
	sha, text := readTLDR(t, body)
	if sha != "abc123" || text != "ships a thing" {
		t.Fatalf("sha = %q, text = %q; want abc123 / ships a thing", sha, text)
	}
}

func TestTLDRRejectsSHAThatWouldCloseTheOpenerComment(t *testing.T) {
	// The SHA is caller-supplied. One containing "-->" would end the opening
	// HTML comment early, so the remainder would render as visible text and
	// the marker structure would be broken. Refuse the write instead.
	for _, sha := range []string{
		"abc123 --> leaked",
		"abc123 <!-- nested",
		"abc123\n<!-- gh-pr-banner:tldr -->",
	} {
		out, err := TLDRApply("existing prose", sha, "a summary", OpSet, PlacementTop)
		if err == nil {
			t.Fatalf("sha %q was accepted; body:\n%s", sha, out.Body)
		}
		if out.Body != "" {
			t.Fatalf("sha %q was rejected but a body came back: %q", sha, out.Body)
		}
	}
}

func TestTLDRSummaryContainingClosingMarkerStaysInsideTheRegion(t *testing.T) {
	// A summary that quotes the closing marker must not close the region
	// early: the banner stays one well-formed region and the text round-trips.
	summary := "explains the fence:\n<!-- /gh-pr-banner:tldr -->\nand why it matters"
	out, err := TLDRApply("pre\n\npost", "abc123", summary, OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	// Exactly one region, still well-formed and still parseable.
	names, err := List(out.Body)
	if err != nil {
		t.Fatalf("the spliced body is malformed: %v", err)
	}
	if len(names) != 1 || names[0] != TLDRName {
		t.Fatalf("names = %v, want [tldr]", names)
	}
	sha, text := readTLDR(t, out.Body)
	if sha != "abc123" || text != summary {
		t.Fatalf("round trip: sha = %q, text = %q, want the summary verbatim", sha, text)
	}
	if !strings.HasPrefix(out.Body, "<!-- gh-pr-banner:tldr") || !strings.HasSuffix(out.Body, "post") {
		t.Fatalf("surrounding prose was lost:\n%s", out.Body)
	}
	// Re-stamping the same summary is still a no-op.
	again, err := TLDRApply(out.Body, "abc123", summary, OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("re-stamp: %v", err)
	}
	if again.Action != ActionUnchanged {
		t.Fatalf("re-stamp action = %s, want unchanged", again.Action)
	}
}

func TestTLDRSummaryContainingOpeningMarkerStaysInsideTheRegion(t *testing.T) {
	// The mirror case: a summary quoting an OPENING marker must not create a
	// second region, which would make the body ambiguous and unwritable.
	summary := "do not write <!-- gh-pr-banner:tldr -->\non its own line"
	out, err := TLDRApply("", "abc123", summary, OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	names, err := List(out.Body)
	if err != nil {
		t.Fatalf("the spliced body is malformed: %v", err)
	}
	if len(names) != 1 {
		t.Fatalf("names = %v, want exactly one region", names)
	}
	_, text := readTLDR(t, out.Body)
	if text != summary {
		t.Fatalf("text = %q, want the summary verbatim", text)
	}
}

// rawTLDRContent returns the region content of the tldr banner in body, the
// way a caller that copies the region out of a pull request body sees it: the
// label line and the quoted summary, without the opener and closer.
func rawTLDRContent(t *testing.T, body string) string {
	t.Helper()
	ok, _, content, err := LookupOpener(body, TLDRName)
	if err != nil || !ok {
		t.Fatalf("lookup: ok=%v err=%v", ok, err)
	}
	return content
}

// assertCleanTLDR checks that body holds exactly one label line, that the
// reader returns want, and that the prose outside the region is intact.
func assertCleanTLDR(t *testing.T, body, want, pre, post string) {
	t.Helper()
	if n := strings.Count(body, "**TLDR**"); n != 1 {
		t.Fatalf("body holds %d TLDR labels, want 1:\n%s", n, body)
	}
	if strings.Contains(body, "> >") {
		t.Fatalf("summary is quoted more than once:\n%s", body)
	}
	if _, text := readTLDR(t, body); text != want {
		t.Fatalf("text = %q, want %q", text, want)
	}
	if !strings.HasPrefix(body, pre) || !strings.HasSuffix(body, post) {
		t.Fatalf("prose outside the region changed:\n%s", body)
	}
}

func TestTLDRSetWithRawRegionContentDoesNotNest(t *testing.T) {
	// A caller that feeds the raw region content of a previous stamp (label
	// line plus quoted summary) back into set gets the clean summary stamped
	// once, not a label nested inside the label.
	pre, post := "pre\n\n", "\n\npost prose"
	body := pre + wantTLDRBlock + post
	fed := rawTLDRContent(t, body)
	out, err := TLDRApply(body, exampleSHA, fed, OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if out.Action != ActionUnchanged {
		t.Fatalf("action = %s, want unchanged", out.Action)
	}
	if out.Body != body {
		t.Fatalf("body changed:\n%s\nwant:\n%s", out.Body, body)
	}
	assertCleanTLDR(t, out.Body, exampleText, pre, post)
}

func TestTLDRSetWithRawRegionContentTwiceDoesNotNest(t *testing.T) {
	// Feeding the region content back twice in a row, under a new SHA, still
	// leaves one label and the clean summary.
	pre, post := "pre\n\n", "\n\npost prose"
	body := pre + wantTLDRBlock + post
	const newSHA = "0123456789abcdef0123456789abcdef01234567"
	for i := 0; i < 2; i++ {
		out, err := TLDRApply(body, newSHA, rawTLDRContent(t, body), OpSet, PlacementTop)
		if err != nil {
			t.Fatalf("set %d: %v", i+1, err)
		}
		body = out.Body
		assertCleanTLDR(t, body, exampleText, pre, post)
	}
	want := pre + strings.Replace(wantTLDRBlock, exampleSHA, newSHA, 1) + post
	if body != want {
		t.Fatalf("body:\n%s\nwant:\n%s", body, want)
	}
}

func TestTLDRSetWithNestedRegionContentRepairsIt(t *testing.T) {
	// Content already nested by an earlier feeding error (a label inside the
	// label) stamps as one label and the clean summary.
	fed := "> **TLDR**\n" +
		"> > **TLDR**\n" +
		"> > > **TLDR**\n" +
		"> > > one line\n" +
		"> > >\n" +
		"> > > second line\n"
	out, err := TLDRApply("", exampleSHA, fed, OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	assertCleanTLDR(t, out.Body, "one line\n\nsecond line", "", "")
}

func TestTLDRSetWithLabelOnlyIsEmptySummary(t *testing.T) {
	// The raw content of an empty tldr is just the label: it stamps as an
	// empty summary, not as a quoted label.
	out, err := TLDRApply("", exampleSHA, TLDRLabel+"\n", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	empty, err := TLDRApply("", exampleSHA, "", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set empty: %v", err)
	}
	if out.Body != empty.Body {
		t.Fatalf("body:\n%s\nwant:\n%s", out.Body, empty.Body)
	}
}

func TestTLDRSetKeepsQuotedSummaryWithoutLabel(t *testing.T) {
	// Only text that starts with the managed label is normalized. A summary
	// that quotes something on its own is the author's text and stays as is.
	const quoted = "> quoted by the author\nplain line"
	out, err := TLDRApply("", exampleSHA, quoted, OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, text := readTLDR(t, out.Body); text != quoted {
		t.Fatalf("text = %q, want %q", text, quoted)
	}
}
