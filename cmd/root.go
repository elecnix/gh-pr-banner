package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/elecnix/gh-pr-banner/internal/banner"
	"github.com/elecnix/gh-pr-banner/internal/ghapi"
)

// Process exit codes. 0 is success; a query finding no banner is 2 — distinct
// from 1 (a real error) so a script can branch on the state.
const (
	ExitOK     = 0
	ExitAbsent = 2
)

// ExitError carries an explicit process exit code so ExecuteOrExit can surface
// "absent" separately from a hard failure.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return fmt.Sprintf("exit %d: %v", e.Code, e.Err) }
func (e *ExitError) Unwrap() error { return e.Err }

func absent() error { return &ExitError{Code: ExitAbsent, Err: errors.New("banner absent")} }

// commonOptions are parsed once and shared by every subcommand.
type commonOptions struct {
	repo   string
	pr     int
	json   bool
	dryRun bool
	at     string

	// resolved once in PersistentPreRunE
	owner, repoName string
	number          int
	url             string
}

// result is emitted (human-prose or JSON) after a command.
type result struct {
	Action  banner.Action `json:"action"`
	Name    string        `json:"name"`
	Repo    string        `json:"repo"`
	PR      int           `json:"pr"`
	URL     string        `json:"url"`
	Present bool          `json:"present"`
	Banner  string        `json:"banner,omitempty"`
	Body    string        `json:"body,omitempty"`
	SHA     string        `json:"sha,omitempty"`
	DryRun  bool          `json:"dry_run,omitempty"`
	Wrote   bool          `json:"wrote,omitempty"`
	Names   []string      `json:"names,omitempty"`
}

func Execute() error {
	return newRootCommand().Execute()
}

func ExecuteOrExit() {
	if err := Execute(); err != nil {
		var ee *ExitError
		if errors.As(err, &ee) {
			fmt.Fprintln(os.Stderr, ee.Err)
			os.Exit(ee.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	o := &commonOptions{at: string(banner.PlacementTop)}
	root := &cobra.Command{
		Use:   "gh pr-banner <set|clear|get|present|list>",
		Short: "Manage delimited banner regions in a pull request body",
		Long: "Deterministically set, update, clear and inspect banner regions in a pull " +
			"request body. A banner is fenced by invisible HTML-comment markers it owns; " +
			"every edit splices that region, byte-preserving everything else — never " +
			"regenerating the body.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	addCommonFlags(root, o)
	// PersistentPreRunE resolves the target once; it runs before the subcommand.
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		owner, repo, explicitRepo, err := ghapi.ResolveRepo(o.repo)
		if err != nil {
			return err
		}
		number, err := ghapi.ResolvePRNumber(owner, repo, explicitRepo, o.pr)
		if err != nil {
			return err
		}
		o.owner, o.repoName, o.number = owner, repo, number
		o.url = ghapi.IssueURL(owner, repo, number)
		return nil
	}
	root.AddCommand(newSetCommand(o))
	root.AddCommand(newClearCommand(o))
	root.AddCommand(newGetCommand(o))
	root.AddCommand(newPresentCommand(o))
	root.AddCommand(newListCommand(o))
	root.AddCommand(newTLDRCommand(o))
	return root
}

func addCommonFlags(cmd *cobra.Command, o *commonOptions) {
	cmd.PersistentFlags().StringVarP(&o.repo, "repo", "R", "", "OWNER/REPO to operate on (default: repository in the current directory)")
	cmd.PersistentFlags().IntVar(&o.pr, "pr", 0, "pull request number (default: PR for the current branch)")
	cmd.PersistentFlags().BoolVar(&o.json, "json", false, "emit machine-readable JSON")
	cmd.PersistentFlags().BoolVar(&o.dryRun, "dry-run", false, "resolve and print what would change without writing to GitHub")
}

// placement returns the parsed banner placement (defaulting to top).
func (o *commonOptions) placement() (banner.Placement, error) {
	if o.at == "" {
		return banner.PlacementTop, nil
	}
	p := banner.Placement(o.at)
	if p != banner.PlacementTop && p != banner.PlacementBottom {
		return "", fmt.Errorf("invalid --at %q (want top or bottom)", o.at)
	}
	return p, nil
}

// emit renders a result in prose or JSON depending on --json.
func (o *commonOptions) emit(r result) {
	if o.json {
		b, _ := json.MarshalIndent(r, "", "  ")
		fmt.Println(string(b))
		return
	}
	switch r.Action {
	case banner.ActionSet:
		note := ""
		if r.DryRun {
			note = " (dry-run; nothing written)"
		}
		fmt.Printf("set banner %q on %s%s\n", r.Name, r.URL, note)
	case banner.ActionUpdated:
		note := ""
		if r.DryRun {
			note = " (dry-run; nothing written)"
		}
		fmt.Printf("updated banner %q on %s%s\n", r.Name, r.URL, note)
	case banner.ActionCleared:
		note := ""
		if r.DryRun {
			note = " (dry-run; nothing written)"
		}
		fmt.Printf("cleared banner %q on %s%s\n", r.Name, r.URL, note)
	case banner.ActionUnchanged:
		fmt.Printf("banner %q already in that state on %s (no change)\n", r.Name, r.URL)
	}
}

// write sets the body and verifies the region landed as intended, failing
// loudly if a concurrent writer clobbered it. newBody is the value computed
// from the pre-read body; expectSet is the exact content the region must hold
// afterwards (empty for a clear, for which the frame must be absent).
func (o *commonOptions) write(expectSet bool, name, expectContent, newBody string) error {
	if _, err := ghapi.PatchBody(o.owner, o.repoName, o.number, newBody); err != nil {
		return err
	}
	fresh, err := ghapi.GetIssue(o.owner, o.repoName, o.number)
	if err != nil {
		return err
	}
	if expectSet {
		ok, content, err := banner.Lookup(fresh.Body, name)
		if err != nil {
			return fmt.Errorf("concurrent modification suspected: body is now malformed after patch: %w", err)
		}
		if !ok {
			return fmt.Errorf("concurrent modification suspected: banner %q is absent after patch", name)
		}
		if content != expectContent {
			return fmt.Errorf("concurrent modification suspected: banner %q was overwritten after patch", name)
		}
	} else {
		ok, _, err := banner.Lookup(fresh.Body, name)
		if err != nil {
			return fmt.Errorf("concurrent modification suspected: body is now malformed after patch: %w", err)
		}
		if ok {
			return fmt.Errorf("concurrent modification suspected: banner %q reappeared after clear", name)
		}
	}
	return nil
}

// fetch gets the current issue/PR document.
func (o *commonOptions) fetch() (ghapi.Issue, error) {
	return ghapi.GetIssue(o.owner, o.repoName, o.number)
}
