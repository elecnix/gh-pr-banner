package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/elecnix/gh-pr-banner/internal/banner"
)

// newTLDRCommand builds `gh pr-banner tldr`, the TLDR banner kind: an
// author-written, human summary of what a pull request ships plus the head SHA
// it describes. The banner carries the SHA; it never enforces freshness (the
// reader compares changed-file sets) and never generates the summary from a
// diff — the value is that a human wrote it.
func newTLDRCommand(o *commonOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tldr <set|get>",
		Short: "Manage the tldr banner kind (summary + the SHA it describes)",
		Long: "A tldr banner is one or two author-written sentences saying what a pull\n" +
			"request ships and why it matters, stored together with the head SHA it\n" +
			"describes. The SHA is metadata for readers (which decide freshness by\n" +
			"comparing changed-file sets); it is never a gate, and nothing here ever\n" +
			"generates the summary — it is always written by a human.",
	}
	cmd.AddCommand(newTLDRSetCommand(o))
	cmd.AddCommand(newTLDRGetCommand(o))
	return cmd
}

func newTLDRSetCommand(o *commonOptions) *cobra.Command {
	var body, bodyFile, sha string
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Create or update the tldr banner (summary + SHA)",
		Long: "Writes the tldr banner: the summary text from --body/--body-file and the\n" +
			"SHA it describes from --sha. The region is spliced like any banner;\n" +
			"everything else in the body is byte-preserved. Setting the same summary\n" +
			"and SHA twice is a no-op (no write to GitHub).",
		Example: "  gh pr-banner tldr set --sha 1a2b3c4 --body \"fixes the flaky retry loop\" --pr 42",
		RunE: func(cmd *cobra.Command, args []string) error {
			if sha == "" {
				return fmt.Errorf("--sha is required: a tldr banner records the head SHA it describes")
			}
			text, err := resolveContent(body, bodyFile)
			if err != nil {
				return err
			}
			content := banner.TLDRContent(sha, text)
			at, err := o.placement()
			if err != nil {
				return err
			}
			is, err := o.fetch()
			if err != nil {
				return err
			}
			out, err := banner.Apply(is.Body, banner.TLDRName, content, banner.OpSet, at)
			if err != nil {
				return err
			}
			res := result{
				Action:  out.Action,
				Name:    banner.TLDRName,
				Repo:    o.owner + "/" + o.repoName,
				PR:      o.number,
				URL:     o.url,
				Present: out.Present,
				Banner:  out.Banner,
				SHA:     sha,
				DryRun:  o.dryRun,
			}
			if out.Action != banner.ActionUnchanged {
				if o.dryRun {
					res.Body = out.Body
					o.emit(res)
				} else {
					if err := o.write(true, banner.TLDRName, content, out.Body); err != nil {
						return err
					}
					res.Wrote = true
					o.emit(res)
				}
			} else {
				o.emit(res)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&sha, "sha", "", "the head SHA this tldr describes (required)")
	cmd.Flags().StringVar(&body, "body", "", "tldr text (mutually exclusive with --body-file)")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "path to a file whose contents become the tldr text (mutually exclusive with --body)")
	cmd.Flags().StringVar(&o.at, "at", string(banner.PlacementTop), "where to place a new region: top or bottom")
	return cmd
}

func newTLDRGetCommand(o *commonOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Print the tldr text and the SHA it describes",
		Long: "Prints the tldr text to stdout. Exits 0 when present; exits 2 (not a hard\n" +
			"error) when absent, so a reader can treat a missing tldr as \"no tldr\".\n" +
			"With --json, prints a JSON object carrying both the text and the SHA.",
		Example: "  gh pr-banner tldr get --pr 42 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			is, err := o.fetch()
			if err != nil {
				return err
			}
			present, content, err := banner.Lookup(is.Body, banner.TLDRName)
			if err != nil {
				return err
			}
			if !present {
				if o.json {
					o.emit(result{
						Action:  "get",
						Name:    banner.TLDRName,
						Repo:    o.owner + "/" + o.repoName,
						PR:      o.number,
						URL:     o.url,
						Present: false,
					})
				}
				return absent()
			}
			tsha, text, err := banner.ParseTLDR(content)
			if err != nil {
				// Present but not in the managed shape: a real error, not a
				// guessing opportunity.
				return err
			}
			if o.json {
				o.emit(result{
					Action:  "get",
					Name:    banner.TLDRName,
					Repo:    o.owner + "/" + o.repoName,
					PR:      o.number,
					URL:     o.url,
					Present: true,
					Banner:  text,
					SHA:     tsha,
				})
			} else {
				fmt.Println(text)
			}
			return nil
		},
	}
}
