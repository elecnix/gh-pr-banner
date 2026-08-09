package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/elecnix/gh-pr-banner/internal/banner"
)

// newSetCommand builds `gh pr-banner set`.
func newSetCommand(o *commonOptions) *cobra.Command {
	var body, bodyFile string
	cmd := &cobra.Command{
		Use:   "set NAME",
		Short: "Create or update a banner region in the pull request body",
		Long: "Writes the banner content into a region fenced by HTML-comment markers\n" +
			"named NAME. If no such region exists it is created; if one exists its content\n" +
			"is replaced. Everything else in the body is byte-preserved. Setting the same\n" +
			"content twice is a no-op (no write to GitHub).",
		Args: func(cmd *cobra.Command, args []string) error {
			return requireExactArgs(cmd, args, 1, "set needs exactly one banner NAME")
		},
		Example: "  gh pr-banner set do-not-merge --body \"staging is red — do not merge\" --pr 42",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			content, err := resolveContent(body, bodyFile)
			if err != nil {
				return err
			}
			at, err := o.placement()
			if err != nil {
				return err
			}
			is, err := o.fetch()
			if err != nil {
				return err
			}
			out, err := banner.Apply(is.Body, name, content, banner.OpSet, at)
			if err != nil {
				return err
			}
			res := result{
				Action:  out.Action,
				Name:    name,
				Repo:    o.owner + "/" + o.repoName,
				PR:      o.number,
				URL:     o.url,
				Present: out.Present,
				Banner:  out.Banner,
				DryRun:  o.dryRun,
			}
			if out.Action != banner.ActionUnchanged {
				if o.dryRun {
					// Nothing written; the region is already correct on GitHub.
					o.emit(res)
				} else {
					if err := o.write(true, name, content, out.Body); err != nil {
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
	cmd.Flags().StringVar(&body, "body", "", "banner content (mutually exclusive with --body-file)")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "path to a file whose contents become the banner (mutually exclusive with --body)")
	cmd.Flags().StringVar(&o.at, "at", string(banner.PlacementTop), "where to place a new region: top or bottom")
	return cmd
}

// resolveContent returns the banner text from exactly one of --body/--body-file.
func resolveContent(body, bodyFile string) (string, error) {
	if body != "" && bodyFile != "" {
		return "", fmt.Errorf("--body and --body-file are mutually exclusive")
	}
	if bodyFile != "" {
		b, err := os.ReadFile(bodyFile)
		if err != nil {
			return "", fmt.Errorf("read --body-file: %w", err)
		}
		return string(b), nil
	}
	if body == "" {
		return "", fmt.Errorf("banner content required: pass --body TEXT or --body-file PATH")
	}
	return body, nil
}

func requireExactArgs(cmd *cobra.Command, args []string, n int, msg string) error {
	if len(args) != n {
		return fmt.Errorf("%s", msg)
	}
	return nil
}
