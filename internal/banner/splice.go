// Package banner implements deterministic, idempotent edits to a delimited
// region of a body of text — the body of a pull request, in practice.
//
// A "banner" is a region fenced by a pair of invisible HTML comments:
//
//	<!-- gh-pr-banner:NAME -->
//
//	<banner content>
//
//	<!-- /gh-pr-banner:NAME -->
//
// set/update/clear operate only on the region between the two markers for a
// given NAME. Everything outside the markers is byte-preserved — the tool never
// regenerates a body, it splices a region.
//
// Safety rules that are load-bearing:
//
//   - Markers are only recognised when they occupy a whole line. A line that
//     merely contains marker-shaped text inline (inside a sentence or a code
//     fence) is ordinary text and is never touched.
//   - A body whose markers are malformed — unbalanced, overlapping, duplicated,
//     or mis-nested — is an error. The tool refuses to guess at a damaged
//     region and changes nothing.
//   - Two separate banners with the same name in one body are ambiguous and
//     rejected. Different names may coexist, and may even be properly nested.
package banner

import (
	"fmt"
	"strings"
)

// Placement controls where a new banner region is inserted when none exists.
type Placement string

const (
	PlacementTop    Placement = "top"
	PlacementBottom Placement = "bottom"
)

// Operation is the edit to perform on a banner region.
type Operation int

const (
	OpSet   Operation = iota // create if absent, replace in place if present
	OpClear                  // remove if present, no-op if absent
)

// Action describes how a body changed as a result of one Apply.
type Action string

const (
	ActionSet       Action = "set"       // the region was created
	ActionUpdated   Action = "updated"   // the existing region's content changed
	ActionCleared   Action = "cleared"   // the region was removed
	ActionUnchanged Action = "unchanged" // no change (set to identical content, or clear of an absent banner)
)

const (
	openPrefix  = "<!-- gh-pr-banner:"
	closePrefix = "<!-- /gh-pr-banner:"
	markerSfx   = " -->"
)

func openMarker(name string) string  { return openPrefix + name + markerSfx }
func closeMarker(name string) string { return closePrefix + name + markerSfx }

// markerName reports whether line, after trimming surrounding whitespace, is a
// whole-line marker formed by prefix+name(+attrs)+suffix, and returns the
// name. An opener may carry ` key: value` attribute pairs between the name
// and the close of the comment (the tldr banner carries its head SHA there);
// closing markers never carry attributes. Names that do not match the
// valid-name shape yield no marker: a line that merely looks marker-shaped but
// is not a canonical marker is ordinary text.
func markerName(line, prefix, suffix string) (string, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, suffix) {
		return "", false
	}
	body := line[len(prefix) : len(line)-len(suffix)]
	if nameRE.MatchString(body) {
		return body, true
	}
	// Attribute form: "<name> <attributes>". The attribute payload is free
	// text carried verbatim — the caller decides what it means.
	if prefix == openPrefix {
		if n, attrs, found := strings.Cut(body, " "); found && nameRE.MatchString(n) && attrs != "" {
			return n, true
		}
	}
	return "", false
}

// Outcome is the result of one Apply.
type Outcome struct {
	Action  Action
	Present bool   // whether the named region is present in the resulting body
	Banner  string // the banner content at the named region ("" when absent)
	Body    string // the resulting body
}

// marker is one recognised marker line.
type marker struct {
	line    int
	name    string
	isClose bool
}

// parseAndValidate scans lines for whole-line markers and checks that the
// entire set of markers forms a well-formed, non-overlapping forest. It fails
// loudly on any malformation rather than guess at a damaged region.
func parseAndValidate(lines []string) ([]marker, error) {
	var ms []marker
	for i, line := range lines {
		if name, ok := markerName(line, openPrefix, markerSfx); ok {
			ms = append(ms, marker{line: i, name: name})
		} else if name, ok := markerName(line, closePrefix, markerSfx); ok {
			ms = append(ms, marker{line: i, name: name, isClose: true})
		}
	}

	// A stack + a per-name open count catches every malformation we care about:
	// close with no open, open with no close, overlapping/mis-nested regions of
	// different names, and duplicate regions of the same name.
	type openRec struct{ name string }
	var stack []openRec
	openCount := map[string]int{}
	for _, m := range ms {
		if !m.isClose {
			stack = append(stack, openRec{m.name})
			openCount[m.name]++
			if openCount[m.name] > 1 {
				return nil, fmt.Errorf("malformed body: banner %q appears in more than one region (second opener at line %d) — refusing to guess which is authoritative", m.name, m.line+1)
			}
			continue
		}
		if len(stack) == 0 {
			return nil, fmt.Errorf("malformed body: closing marker for %q at line %d has no opening marker", m.name, m.line+1)
		}
		top := stack[len(stack)-1]
		if top.name != m.name {
			return nil, fmt.Errorf("malformed body: closing marker for %q at line %d crosses the open region of %q", m.name, m.line+1, top.name)
		}
		stack = stack[:len(stack)-1]
	}
	if len(stack) > 0 {
		// Find which opener is unclosed for a precise message.
		var unclosed []string
		for _, r := range stack {
			unclosed = append(unclosed, r.name)
		}
		return nil, fmt.Errorf("malformed body: banner region(s) never closed: %s", strings.Join(unclosed, ", "))
	}
	return ms, nil
}

// regionFor returns the inclusive line range [openLine, closeLine] of the
// single region owned by name, and whether it exists. callers must have already
// validated the markers.
func regionFor(ms []marker, name string) (openLine, closeLine int, ok bool) {
	for _, m := range ms {
		if m.name != name {
			continue
		}
		if !m.isClose {
			openLine = m.line
		} else {
			return openLine, m.line, true
		}
	}
	return 0, 0, false
}

// blockLines renders the banner region for name with the given content as the
// set of lines that will own the region in the destination body. open is the
// full opener line (a caller-supplied one for ApplyOpener). hugOpener: an
// opener-carried attribute makes the first content line managed payload that
// hugs the opener (no blank separator) — the tldr banner renders its label
// immediately under the opener comment that carries the SHA.
func blockLines(name, open, content string, hugOpener bool) []string {
	content = strings.TrimSpace(content)
	block := []string{open}
	if content == "" {
		return append(block, closeMarker(name))
	}
	if !hugOpener {
		block = append(block, "")
	}
	block = append(block, strings.Split(content, "\n")...)
	return append(block, "", closeMarker(name))
}

// joinLines reassembles lines into a body, stripping trailing blank lines that
// the splice itself introduced so the result is tidy without touching any
// unmanaged leading content.
func joinLines(lines []string) string {
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// sameBody reports whether a and b differ only in trailing newlines — the only
// axis on which two equivalent spliced bodies may differ without meaning.
func sameBody(a, b string) bool {
	return strings.TrimRight(a, "\n") == strings.TrimRight(b, "\n")
}

// Apply performs op on the banner named `name` in currentBody.
//
//   - OpSet writes `content` into the region: create it at `at` if absent,
//     replace it in place if present. If the result is byte-identical to the
//     input (set to the same content), Action is Unchanged and Body equals the
//     input — the caller can skip the network write.
//   - OpClear removes the region if present, and is a quiet no-op (Action
//     Cleared, Body unchanged) if absent.
//
// A body with malformed markers is an error and nothing is changed.
func Apply(currentBody, name, content string, op Operation, at Placement) (Outcome, error) {
	if err := ValidateName(name); err != nil {
		return Outcome{}, err
	}
	if at != PlacementTop && at != PlacementBottom {
		return Outcome{}, fmt.Errorf("invalid placement %q (want %q or %q)", at, PlacementTop, PlacementBottom)
	}
	want := strings.TrimSpace(content)
	return splice(currentBody, name, func(open, existing string) (string, []string, error) {
		if existing == want {
			return "", nil, errUnchanged
		}
		return want, blockLines(name, open, want, false), nil
	}, op, at)
}

// ApplyOpener is Apply with a caller-supplied opener: the region's opening
// marker line is written verbatim as `opener` instead of the bare-name form,
// so a banner kind that carries metadata in its opening comment (the tldr
// banner carries the head SHA there) keeps it across every rewrite. OpClear
// ignores the opener and behaves exactly like Apply's.
func ApplyOpener(currentBody, name, opener, content string, op Operation, at Placement) (Outcome, error) {
	if err := ValidateName(name); err != nil {
		return Outcome{}, err
	}
	if at != PlacementTop && at != PlacementBottom {
		return Outcome{}, fmt.Errorf("invalid placement %q (want %q or %q)", at, PlacementTop, PlacementBottom)
	}
	opener = strings.TrimSpace(opener)
	if op == OpSet && (!strings.HasPrefix(opener, openPrefix+name) || !strings.HasSuffix(opener, markerSfx)) {
		return Outcome{}, fmt.Errorf("internal error: opener %q does not open a %q banner", opener, name)
	}
	want := strings.TrimSpace(content)
	// An opener that carries attributes after the bare name hugs its content:
	// the first line is managed payload (the tldr label), not free prose.
	hugOpener := strings.HasPrefix(opener, openPrefix+name+" ")
	return splice(currentBody, name, func(open, existing string) (string, []string, error) {
		if existing == want && open == opener {
			return "", nil, errUnchanged
		}
		return want, blockLines(name, opener, want, hugOpener), nil
	}, op, at)
}

// errUnchanged is the sentinel a region builder returns when the region
// already holds exactly what was asked for.
var errUnchanged = fmt.Errorf("unchanged")

// splice is the shared deterministic edit behind Apply and ApplyOpener.
// build receives the region's existing opener line and content ("" when
// absent) and either returns errUnchanged (the region already holds what was
// asked for) or the content and rendered lines the region should hold.
func splice(currentBody, name string, build func(open, existing string) (string, []string, error), op Operation, at Placement) (Outcome, error) {
	lines := strings.Split(currentBody, "\n")
	ms, err := parseAndValidate(lines)
	if err != nil {
		return Outcome{}, err
	}
	o, c, present := regionFor(ms, name)
	existingOpen, existingContent := openMarker(name), ""
	if present {
		existingOpen = strings.TrimSpace(lines[o])
		existingContent = strings.TrimSpace(strings.Join(lines[o+1:c], "\n"))
	}

	switch op {
	case OpSet:
		content, block, err := build(existingOpen, existingContent)
		if err == errUnchanged {
			return Outcome{Action: ActionUnchanged, Present: present, Banner: existingContent, Body: currentBody}, nil
		}
		if err != nil {
			return Outcome{}, err
		}
		if present {
			newLines := append(append([]string{}, lines[:o]...), block...)
			newLines = append(newLines, lines[c+1:]...)
			out := Outcome{Present: true, Banner: content, Body: joinLines(newLines)}
			if sameBody(currentBody, out.Body) {
				out.Action = ActionUnchanged
				out.Body = currentBody
			} else {
				out.Action = ActionUpdated
			}
			return out, nil
		}
		var newLines []string
		switch at {
		case PlacementTop:
			newLines = append(newLines, block...)
			newLines = append(newLines, "")
			if len(lines) > 0 && lines[0] == "" {
				lines = lines[1:] // avoid a double blank under the new region separator
			}
			newLines = append(newLines, lines...)
		case PlacementBottom:
			if len(lines) > 0 && lines[len(lines)-1] == "" {
				lines = lines[:len(lines)-1] // avoid a double blank before the new region
			}
			newLines = append(newLines, lines...)
			newLines = append(newLines, "")
			newLines = append(newLines, block...)
		default:
			return Outcome{}, fmt.Errorf("invalid placement %q (want %q or %q)", at, PlacementTop, PlacementBottom)
		}
		return Outcome{Action: ActionSet, Present: true, Banner: content, Body: joinLines(newLines)}, nil

	case OpClear:
		if !present {
			return Outcome{Action: ActionCleared, Present: false, Body: currentBody}, nil
		}
		newLines := append([]string{}, lines[:o]...)
		newLines = append(newLines, lines[c+1:]...)
		// Drop blank lines left over where the (managed) region sat. These only
		// ever come from our own region separators, never from unmanaged content.
		for len(newLines) > 0 && newLines[0] == "" {
			newLines = newLines[1:]
		}
		for len(newLines) > 0 && newLines[len(newLines)-1] == "" {
			newLines = newLines[:len(newLines)-1]
		}
		return Outcome{Action: ActionCleared, Present: false, Body: joinLines(newLines)}, nil
	}
	return Outcome{}, fmt.Errorf("unknown operation %v", op)
}

// Lookup reports whether a banner named `name` is present in body and, if so,
// its content. It applies the same malformed-body validation as Apply.
func Lookup(body, name string) (present bool, content string, err error) {
	return lookup(body, name)
}

// LookupOpener reports whether a banner named `name` is present in body and,
// if so, its opener line and content. A banner kind that carries metadata in
// its opener reads it back here.
func LookupOpener(body, name string) (present bool, opener, content string, err error) {
	lines := strings.Split(body, "\n")
	ms, err := parseAndValidate(lines)
	if err != nil {
		return false, "", "", err
	}
	o, c, ok := regionFor(ms, name)
	if !ok {
		return false, "", "", nil
	}
	return true, strings.TrimSpace(lines[o]), strings.TrimSpace(strings.Join(lines[o+1:c], "\n")), nil
}

func lookup(body, name string) (bool, string, error) {
	ok, _, content, err := LookupOpener(body, name)
	return ok, content, err
}

// List returns the names of every banner present in body, in first-appearance
// order. It applies the same malformed-body validation as Apply.
func List(body string) ([]string, error) {
	lines := strings.Split(body, "\n")
	ms, err := parseAndValidate(lines)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ms))
	seen := map[string]bool{}
	for _, m := range ms {
		if !m.isClose && !seen[m.name] {
			names = append(names, m.name)
			seen[m.name] = true
		}
	}
	return names, nil
}
