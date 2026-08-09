package banner

import (
	"strings"
	"testing"
)

func TestInsertIntoEmptyBody(t *testing.T) {
	out, err := Apply("", "do-not-merge", "Do not merge — staging is red", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set into empty body: %v", err)
	}
	if out.Action != ActionSet {
		t.Fatalf("action = %s, want set", out.Action)
	}
	if !out.Present {
		t.Fatal("present = false, want true")
	}
	want := "<!-- gh-pr-banner:do-not-merge -->\n\nDo not merge — staging is red\n\n<!-- /gh-pr-banner:do-not-merge -->"
	if out.Body != want {
		t.Fatalf("body:\n%s\nwant:\n%s", out.Body, want)
	}
}

func TestInsertAlongsideExistingContent(t *testing.T) {
	body := "# Title\n\nSome existing description.\n\n## Details\nmore"
	out, err := Apply(body, "do-not-merge", "STAGING RED", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set alongside: %v", err)
	}
	if out.Action != ActionSet {
		t.Fatalf("action = %s, want set", out.Action)
	}
	// Banner on top, original content byte-preserved below.
	if !strings.HasPrefix(out.Body, "<!-- gh-pr-banner:do-not-merge -->\n\nSTAGING RED\n\n<!-- /gh-pr-banner:do-not-merge -->") {
		t.Fatalf("banner not inserted at top:\n%s", out.Body)
	}
	if !strings.Contains(out.Body, "# Title\n\nSome existing description.\n\n## Details\nmore") {
		t.Fatalf("original content not preserved:\n%s", out.Body)
	}
}

func TestInsertBottom(t *testing.T) {
	body := "lead line\nrest"
	out, err := Apply(body, "build-status", "green", OpSet, PlacementBottom)
	if err != nil {
		t.Fatalf("set bottom: %v", err)
	}
	if !strings.HasPrefix(out.Body, "lead line\nrest") {
		t.Fatalf("original lost:\n%s", out.Body)
	}
	if !strings.HasSuffix(out.Body, "\n<!-- gh-pr-banner:build-status -->\n\ngreen\n\n<!-- /gh-pr-banner:build-status -->") {
		t.Fatalf("banner not appended:\n%s", out.Body)
	}
}

func TestUpdateInPlace(t *testing.T) {
	body := "pre\n<!-- gh-pr-banner:do-not-merge -->\n\nSTAGING RED\n\n<!-- /gh-pr-banner:do-not-merge -->\npost"
	out, err := Apply(body, "do-not-merge", "STAGING GREEN", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if out.Action != ActionUpdated {
		t.Fatalf("action = %s, want updated", out.Action)
	}
	want := "pre\n<!-- gh-pr-banner:do-not-merge -->\n\nSTAGING GREEN\n\n<!-- /gh-pr-banner:do-not-merge -->\npost"
	if out.Body != want {
		t.Fatalf("body:\n%s\nwant:\n%s", out.Body, want)
	}
}

func TestSetToSameContentIsUnchanged(t *testing.T) {
	body := "pre\n<!-- gh-pr-banner:do-not-merge -->\n\nSTAGING RED\n\n<!-- /gh-pr-banner:do-not-merge -->\npost"
	out, err := Apply(body, "do-not-merge", "STAGING RED", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set same: %v", err)
	}
	if out.Action != ActionUnchanged {
		t.Fatalf("action = %s, want unchanged (idempotent)", out.Action)
	}
	if out.Body != body {
		t.Fatalf("body changed on no-op set:\n%s", out.Body)
	}
}

func TestClear(t *testing.T) {
	body := "pre\n<!-- gh-pr-banner:do-not-merge -->\n\nSTAGING RED\n\n<!-- /gh-pr-banner:do-not-merge -->\npost"
	out, err := Apply(body, "do-not-merge", "", OpClear, PlacementTop)
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if out.Action != ActionCleared {
		t.Fatalf("action = %s, want cleared", out.Action)
	}
	if out.Present {
		t.Fatal("present = true after clear")
	}
	want := "pre\npost"
	if out.Body != want {
		t.Fatalf("body:\n%q\nwant:\n%q", out.Body, want)
	}
}

func TestClearAbsentIsNoop(t *testing.T) {
	body := "just some content, no banners"
	out, err := Apply(body, "missing", "", OpClear, PlacementTop)
	if err != nil {
		t.Fatalf("clear absent: %v", err)
	}
	if out.Action != ActionCleared {
		t.Fatalf("action = %s, want cleared (quiet no-op)", out.Action)
	}
	if out.Body != body {
		t.Fatalf("body changed on clear-of-absent:\n%s", out.Body)
	}
}

func TestMultipleBannersCoexistAndClearIndependently(t *testing.T) {
	body := "top"
	out, err := Apply(body, "do-not-merge", "RED", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set a: %v", err)
	}
	out, err = Apply(out.Body, "needs-code-owner", "yes", OpSet, PlacementBottom)
	if err != nil {
		t.Fatalf("set b: %v", err)
	}
	if got := countMarker(out.Body, "do-not-merge"); got != 1 {
		t.Fatalf("do-not-merge markers = %d, want 1", got)
	}
	if got := countMarker(out.Body, "needs-code-owner"); got != 1 {
		t.Fatalf("needs-code-owner markers = %d, want 1", got)
	}
	// Clear one; the other survives.
	cleared, err := Apply(out.Body, "do-not-merge", "", OpClear, PlacementTop)
	if err != nil {
		t.Fatalf("clear a: %v", err)
	}
	if countMarker(cleared.Body, "do-not-merge") != 0 {
		t.Fatal("do-not-merge not cleared")
	}
	if countMarker(cleared.Body, "needs-code-owner") != 1 {
		t.Fatal("needs-code-owner was cleared too")
	}
}

func TestProperlyNestedDistinctBannersAllowed(t *testing.T) {
	// Distinct names may nest cleanly (stack-balanced). This is not a
	// malformation — only mis-nesting is.
	body := "<!-- gh-pr-banner:outer -->\n\n<!-- gh-pr-banner:inner -->\n\nx\n\n<!-- /gh-pr-banner:inner -->\n\n<!-- /gh-pr-banner:outer -->"
	out, err := Apply(body, "inner", "y", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set inside nested region: %v", err)
	}
	if out.Action != ActionUpdated {
		t.Fatalf("action = %s, want updated", out.Action)
	}
}

func TestMalformedUnclosedRegion(t *testing.T) {
	body := "x\n<!-- gh-pr-banner:a -->\ny"
	_, err := Apply(body, "a", "z", OpSet, PlacementTop)
	if err == nil {
		t.Fatal("expected error for unclosed region, got nil")
	}
	if !strings.Contains(err.Error(), "never closed") {
		t.Fatalf("error = %q, want mention of never closed", err.Error())
	}
}

func TestMalformedCloseWithoutOpen(t *testing.T) {
	body := "x\n<!-- /gh-pr-banner:a -->\ny"
	_, err := Apply(body, "a", "z", OpSet, PlacementTop)
	if err == nil {
		t.Fatal("expected error for close-without-open, got nil")
	}
}

func TestMalformedOverlappingRegions(t *testing.T) {
	body := "<!-- gh-pr-banner:a -->\n<!-- gh-pr-banner:b -->\n<!-- /gh-pr-banner:a -->\n<!-- /gh-pr-banner:b -->"
	_, err := Apply(body, "a", "z", OpSet, PlacementTop)
	if err == nil {
		t.Fatal("expected error for overlapping regions, got nil")
	}
	if !strings.Contains(err.Error(), "crosses") {
		t.Fatalf("error = %q, want mention of crossing", err.Error())
	}
}

func TestMalformedDuplicateRegionSameName(t *testing.T) {
	body := "<!-- gh-pr-banner:a -->\nx\n<!-- /gh-pr-banner:a -->\n<!-- gh-pr-banner:a -->\ny\n<!-- /gh-pr-banner:a -->"
	_, err := Apply(body, "a", "z", OpSet, PlacementTop)
	if err == nil {
		t.Fatal("expected error for duplicate region, got nil")
	}
	if !strings.Contains(err.Error(), "more than one region") {
		t.Fatalf("error = %q, want duplicate-region mention", err.Error())
	}
}

func TestPhantomInlineMarkerIgnored(t *testing.T) {
	// A line that merely contains marker-shaped text inline (the classic
	// "the rewriter left an example in the body") is NOT a marker. It must be
	// preserved byte-for-byte and must not trip validation.
	body := "see also <!-- gh-pr-banner:do-not-merge --> inside a sentence\nrest"
	out, err := Apply(body, "do-not-merge", "RED", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set with inline phantom present: %v", err)
	}
	if !strings.Contains(out.Body, "see also <!-- gh-pr-banner:do-not-merge --> inside a sentence") {
		t.Fatalf("inline phantom text was not preserved:\n%s", out.Body)
	}
	// It must not have been counted as an existing region: setting the same
	// name inserts a fresh one at top rather than updating in place.
	if !strings.HasPrefix(out.Body, "<!-- gh-pr-banner:do-not-merge -->\n\nRED\n\n<!-- /gh-pr-banner:do-not-merge -->\n\nsee also") {
		t.Fatalf("inline phantom was treated as a real region:\n%s", out.Body)
	}
}

func TestPhantomInvalidNameMarkerIgnored(t *testing.T) {
	// Uppercase / spaced "markers" are not canonical and are left alone.
	body := "<!-- gh-pr-banner:Do-Not-Merge -->\nrest"
	out, err := Apply(body, "do-not-merge", "RED", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set with invalid-name phantom present: %v", err)
	}
	if !strings.Contains(out.Body, "<!-- gh-pr-banner:Do-Not-Merge -->") {
		t.Fatal("invalid-name phantom was mangled")
	}
}

func TestLookupPresent(t *testing.T) {
	body := "a\n<!-- gh-pr-banner:do-not-merge -->\n\nSTAGING RED\n\n<!-- /gh-pr-banner:do-not-merge -->\nb"
	ok, content, err := Lookup(body, "do-not-merge")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if !ok {
		t.Fatal("present = false, want true")
	}
	if content != "STAGING RED" {
		t.Fatalf("content = %q, want STAGING RED", content)
	}
}

func TestLookupAbsent(t *testing.T) {
	ok, content, err := Lookup("no banners here", "missing")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if ok {
		t.Fatal("present = true, want false")
	}
	if content != "" {
		t.Fatalf("content = %q, want empty", content)
	}
}

func TestList(t *testing.T) {
	valid := "<!-- gh-pr-banner:alpha -->\na\n<!-- /gh-pr-banner:alpha -->\n<!-- gh-pr-banner:beta -->\nb\n<!-- /gh-pr-banner:beta -->"
	names, err := List(valid)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("names = %v, want [alpha beta]", names)
	}
}

func TestNameValidation(t *testing.T) {
	for _, bad := range []string{"", "UPPER", "has space", "with/slash", "12345678901234567890123456789012345678901234567890123456789012345"} {
		if err := ValidateName(bad); err == nil {
			t.Fatalf("expected error for name %q", bad)
		}
	}
	for _, good := range []string{"a", "do-not-merge", "needs-code-owner", "x1-2"} {
		if err := ValidateName(good); err != nil {
			t.Fatalf("unexpected error for name %q: %v", good, err)
		}
	}
}

func countMarker(body, name string) int {
	return strings.Count(body, openMarker(name))
}
