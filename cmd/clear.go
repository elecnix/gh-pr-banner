package cmd

import (
	"github.com/spf13/cobra"

	"github.com/elecnix/gh-pr-banner/internal/banner"
)

func newClearCommand(o *commonOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "clear NAME",
		Short: "Remove a banner region from the pull request body",
		Long: "Removes the region owned by NAME. If the banner is absent this succeeds\n" +
			"quietly (no write, exit 0), so a script can clear without first checking.",
		Args: func(cmd *cobra.Command, args []string) error {
			return requireExactArgs(cmd, args, 1, "clear needs exactly one banner NAME")
		},
		Example: "  gh pr-banner clear do-not-merge --pr 42",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := banner.ValidateName(name); err != nil {
				return err
			}
			is, err := o.fetch()
			if err != nil {
				return err
			}
			out, err := banner.Apply(is.Body, name, "", banner.OpClear, banner.PlacementTop)
			if err != nil {
				return err
			}
			res := result{
				Action: out.Action,
				Name:   name,
				Repo:   o.owner + "/" + o.repoName,
				PR:     o.number,
				URL:    o.url,
				DryRun: o.dryRun,
			}
			if out.Action == banner.ActionCleared && out.Body != is.Body && !o.dryRun {
				if err := o.write(false, name, "", out.Body); err != nil {
					return err
				}
				res.Wrote = true
			}
			o.emit(res)
			return nil
		},
	}
}
