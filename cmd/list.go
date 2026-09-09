package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/elecnix/gh-pr-banner/internal/banner"
)

// newListCommand builds `gh pr-banner list`.
func newListCommand(o *commonOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the banner names present in the pull request body",
		Long: "Prints the name of every banner region present in the body, one per line.\n" +
			"With --json, prints {\"names\": [...]}.",
		Args:    cobra.NoArgs,
		Example: "  gh pr-banner list --pr 42",
		RunE: func(cmd *cobra.Command, args []string) error {
			is, err := o.fetch()
			if err != nil {
				return err
			}
			names, err := banner.List(is.Body)
			if err != nil {
				return err
			}
			if o.json {
				o.emit(result{
					Action: banner.ActionUnchanged,
					Name:   "",
					Repo:   o.repoLabel(),
					PR:     o.number,
					URL:    o.url,
					Names:  names,
				})
				return nil
			}
			for _, n := range names {
				fmt.Println(n)
			}
			return nil
		},
	}
}
