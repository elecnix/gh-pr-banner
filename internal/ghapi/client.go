// Package ghapi is a thin client over the GitHub REST API, reached through the
// `gh` CLI so the operator's existing authentication and host configuration
// (hosts.yml, identity files) apply with no extra setup. It only needs the
// four operations this extension performs: resolve the repo, resolve the PR
// number, read the issue/PR body, and patch the body back.
package ghapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Issue is the slice of an issue/PR document this tool reads.
type Issue struct {
	Body      string `json:"body"`
	HTMLURL   string `json:"html_url"`
	State     string `json:"state"`
	UpdatedAt string `json:"updated_at"`
}

// Repo describes a repository.
type Repo struct {
	NameWithOwner string `json:"nameWithOwner"`
}

// Error is a failed `gh` invocation, carrying the decoded HTTP status when one
// is available so callers can branch on the real cause rather than a generic
// "exit status 1".
type Error struct {
	StatusCode int
	Msg        string
	Stderr     string
	Err        error
}

func (e *Error) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("gh api error (status %d): %s", e.StatusCode, e.Msg)
	}
	return fmt.Sprintf("gh error: %s", e.Msg)
}

func (e *Error) Unwrap() error { return e.Err }

// run executes `gh args...` and returns stdout. On failure it returns an Error
// that preserves the decoded HTTP status code when present.
func run(args ...string) ([]byte, error) {
	return runInput(nil, args...)
}

// runInput executes `gh args...`, feeding stdin for commands that use
// `--input -` (here: PATCHing the issue body).
func runInput(stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		status := 0
		// gh renders HTTP errors as "…: HTTP 422" or "gh: Not Found (HTTP 404)".
		if idx := strings.Index(msg, "(HTTP "); idx >= 0 {
			if code, cerr := strconv.Atoi(strings.TrimSuffix(msg[idx+len("(HTTP "):], ")")); cerr == nil {
				status = code
			}
		}
		return out.Bytes(), &Error{StatusCode: status, Msg: msg, Stderr: errOut.String(), Err: err}
	}
	return out.Bytes(), nil
}

// ResolveRepo turns a possibly-empty `-R OWNER/REPO` argument and the working
// directory into (owner, repo). When repoArg is empty it asks gh for the repo
// containing the current directory.
func ResolveRepo(repoArg string) (string, string, error) {
	if repoArg != "" {
		parts := strings.SplitN(repoArg, "/", 3)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", "", fmt.Errorf("invalid --repo %q: want OWNER/REPO", repoArg)
		}
		return parts[0], parts[1], nil
	}
	out, err := run("repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	if err != nil {
		return "", "", fmt.Errorf("detect repository: %w", err)
	}
	owner, repo, e := splitNameWithOwner(strings.TrimSpace(string(out)))
	if e != nil {
		return "", "", fmt.Errorf("detect repository: %w", e)
	}
	return owner, repo, nil
}

// ResolvePRNumber returns the PR/issue number: the given number when non-zero,
// otherwise the number of the PR open for the current branch in owner/repo.
func ResolvePRNumber(owner, repo string, number int) (int, error) {
	if number > 0 {
		return number, nil
	}
	out, err := run("pr", "view", "-R", owner+"/"+repo, "--json", "number", "--jq", ".number")
	if err != nil {
		return 0, fmt.Errorf("no --pr given and no PR for the current branch in %s/%s: %w", owner, repo, err)
	}
	n, cerr := strconv.Atoi(strings.TrimSpace(string(out)))
	if cerr != nil {
		return 0, fmt.Errorf("parse PR number %q: %w", strings.TrimSpace(string(out)), cerr)
	}
	return n, nil
}

// GetIssue fetches the issue/PR document for owner/repo#number.
func GetIssue(owner, repo string, number int) (Issue, error) {
	var is Issue
	out, err := run("api", "repos/"+owner+"/"+repo+"/issues/"+strconv.Itoa(number))
	if err != nil {
		return is, err
	}
	if err := json.Unmarshal(out, &is); err != nil {
		return is, fmt.Errorf("parse issue response: %w", err)
	}
	return is, nil
}

// PatchBody sets the issue/PR body and returns the resulting document.
func PatchBody(owner, repo string, number int, newBody string) (Issue, error) {
	payload, err := json.Marshal(map[string]string{"body": newBody})
	if err != nil {
		return Issue{}, fmt.Errorf("marshal body: %w", err)
	}
	var is Issue
	out, err := runInput(payload, "api", "repos/"+owner+"/"+repo+"/issues/"+strconv.Itoa(number), "-X", "PATCH", "--input", "-")
	if err != nil {
		return is, err
	}
	if err := json.Unmarshal(out, &is); err != nil {
		return is, fmt.Errorf("parse patch response: %w", err)
	}
	return is, nil
}

// IssueURL builds the canonical HTML URL for owner/repo#number.
func IssueURL(owner, repo string, number int) string {
	return "https://github.com/" + owner + "/" + repo + "/pull/" + strconv.Itoa(number)
}

func splitNameWithOwner(nwo string) (string, string, error) {
	parts := strings.SplitN(nwo, "/", 3)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("unexpected nameWithOwner %q", nwo)
	}
	return parts[0], parts[1], nil
}
