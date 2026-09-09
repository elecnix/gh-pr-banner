package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/elecnix/gh-pr-banner/internal/banner"
)

func newGetCommand(o *commonOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "get NAME",
		Short: "Print the content of a banner region",
		Long: "Prints the current content of the region owned by NAME to stdout. Exits 0\n" +
			"when present; exits 2 (not a hard error) when absent, so a caller can branch\n" +
			"on the state without parsing the body. With --json, prints a JSON object.",
		Args: func(cmd *cobra.Command, args []string) error {
			return requireExactArgs(cmd, args, 1, "get needs exactly one banner NAME")
		},
		Example: "  gh pr-banner get do-not-merge --pr 42 && echo merge-safe || true",
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
			if present {
				if o.json {
					o.emit(result{
						Action:  "get",
						Name:    name,
						Repo:    o.repoLabel(),
						PR:      o.number,
						URL:     o.url,
						Present: true,
						Banner:  content,
					})
				} else {
					fmt.Println(content)
				}
				return nil
			}
			// Absent: print nothing to stdout (the exit code is the signal). JSON
			// callers still get the state object.
			if o.json {
				o.emit(result{
					Action:  "get",
					Name:    name,
					Repo:    o.repoLabel(),
					PR:      o.number,
					URL:     o.url,
					Present: false,
				})
			}
			return absent()
		},
	}
}
