package cmd

import (
	"github.com/spf13/cobra"

	"github.com/elecnix/gh-pr-banner/internal/banner"
)

// newPresentCommand builds `gh pr-banner present`, the script-friendly boolean
// probe: exit 0 when the banner is present, 2 when absent, printing nothing.
func newPresentCommand(o *commonOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "present NAME",
		Short: "Exit 0 if a banner is present, 2 if absent (for scripts)",
		Long: "A boolean probe: exits 0 when the region owned by NAME is present and 2 when\n" +
			"absent, printing nothing to stdout. With --json, also prints a JSON object.",
		Args: func(cmd *cobra.Command, args []string) error {
			return requireExactArgs(cmd, args, 1, "present needs exactly one banner NAME")
		},
		Example: "gh pr-banner present do-not-merge --pr 42",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := banner.ValidateName(name); err != nil {
				return err
			}
			is, err := o.fetch()
			if err != nil {
				return err
			}
			present, content, err := banner.Lookup(is.Body, name)
			if err != nil {
				return err
			}
			if !present {
				if o.json {
					o.emit(result{
						Action:  banner.ActionUnchanged,
						Name:    name,
						Repo:    o.owner + "/" + o.repoName,
						PR:      o.number,
						URL:     o.url,
						Present: false,
					})
				}
				return absent()
			}
			if o.json {
				o.emit(result{
					Action:  banner.ActionUnchanged,
					Name:    name,
					Repo:    o.owner + "/" + o.repoName,
					PR:      o.number,
					URL:     o.url,
					Present: true,
					Banner:  content,
				})
			}
			return nil
		},
	}
}
