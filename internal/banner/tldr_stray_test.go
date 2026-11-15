package banner

import (
	"strings"
	"testing"
)

// A "> **TLDR**" label is managed payload: the writer emits it and the parser
// strips it. A body that carries a second one, typed by hand outside the
// region, renders the same summary shape twice under two bold labels. Setting
// a banner over such a body must refuse and name the line, because the tool
// never rewrites prose it did not write.

func TestTLDRSetRefusesAHandWrittenLabel(t *testing.T) {
	body := "> **TLDR**\n" +
		"> A summary someone typed by hand instead of using the banner.\n" +
		"\n" +
		"The change itself, in ordinary paragraphs.\n"
	out, err := TLDRApply(body, exampleSHA, "one line", OpSet, PlacementTop)
	if err == nil {
		t.Fatalf("set over a hand-written label succeeded; body:\n%s", out.Body)
	}
	if out.Body != "" {
		t.Fatalf("a refused set returned a body:\n%s", out.Body)
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("error does not name the line: %v", err)
	}
	if !strings.Contains(err.Error(), TLDRLabel) {
		t.Fatalf("error does not name the label: %v", err)
	}
}

func TestTLDRSetRefusesALabelBelowTheRegion(t *testing.T) {
	once, err := TLDRApply("Intro paragraph.\n", exampleSHA, "one line", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("first stamp: %v", err)
	}
	body := once.Body + "\n\n> **TLDR**\n> The summary typed a second time by hand.\n"
	if _, err := TLDRApply(body, exampleSHA, "one line", OpSet, PlacementTop); err == nil {
		t.Fatal("set over a body with a second label below the region succeeded")
	}
}

func TestTLDRSetRefusesWhitespaceVariantsOfTheLabel(t *testing.T) {
	body := ">  **TLDR**  \n> Summary typed by hand.\n\nProse.\n"
	if _, err := TLDRApply(body, exampleSHA, "one line", OpSet, PlacementTop); err == nil {
		t.Fatal("set over a spaced label variant succeeded")
	}
}

func TestTLDRSetAllowsALabelInsideAFence(t *testing.T) {
	body := "Prose.\n\n```markdown\n> **TLDR**\n> example of the rendered banner\n```\n"
	out, err := TLDRApply(body, exampleSHA, "one line", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("set over a fenced example label: %v", err)
	}
	if !strings.Contains(out.Body, "> **TLDR**\n> example of the rendered banner") {
		t.Fatalf("the fenced example was damaged:\n%s", out.Body)
	}
}

func TestTLDRSetAllowsItsOwnLabel(t *testing.T) {
	// The region's own label is legitimate: re-stamping stays a no-op.
	once, err := TLDRApply("Prose.\n", exampleSHA, "one line", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("first stamp: %v", err)
	}
	twice, err := TLDRApply(once.Body, exampleSHA, "one line", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("re-stamp over the managed label: %v", err)
	}
	if twice.Action != ActionUnchanged {
		t.Fatalf("action = %s, want unchanged", twice.Action)
	}
}

func TestTLDRClearStillWorksBesideAStrayLabel(t *testing.T) {
	// Clearing must never be blocked: it removes only the managed region.
	once, err := TLDRApply("", exampleSHA, "one line", OpSet, PlacementTop)
	if err != nil {
		t.Fatalf("stamp: %v", err)
	}
	body := once.Body + "\n\n> **TLDR**\n> hand-written\n"
	out, err := TLDRApply(body, "", "", OpClear, PlacementTop)
	if err != nil {
		t.Fatalf("clear beside a stray label: %v", err)
	}
	if out.Present {
		t.Fatalf("region still present after clear:\n%s", out.Body)
	}
	if !strings.Contains(out.Body, "> **TLDR**\n> hand-written") {
		t.Fatalf("clear damaged the hand-written text:\n%s", out.Body)
	}
}

func TestTLDRSetSeesAStrayLabelAfterAFencedOpener(t *testing.T) {
	// A fenced example of the whole region is documentation, not a region: it
	// must not hide a hand-written label that follows it.
	body := "Prose.\n\n" +
		"```md\n" +
		"<!-- gh-pr-banner:tldr tldr-head-sha: abc123 -->\n" +
		"> **TLDR**\n" +
		"> example\n" +
		"<!-- /gh-pr-banner:tldr -->\n" +
		"```\n\n" +
		"> **TLDR**\n" +
		"> typed by hand after the example\n"
	out, err := TLDRApply(body, exampleSHA, "one line", OpSet, PlacementTop)
	if err == nil {
		t.Fatalf("set over a fenced opener followed by a real label succeeded; body:\n%s", out.Body)
	}
	if !strings.Contains(err.Error(), "line 10") {
		t.Fatalf("error does not name the stray label's line: %v", err)
	}
}
