package cli

import (
	"github.com/spf13/cobra"

	"github.com/rafaelfragoso/columbus/internal/project"
)

func newInitCmd(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a Columbus project in the current directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := project.Init(project.InitParams{
				WorkDir: env.WorkDir,
				IDs:     env.IDs,
				Getenv:  env.Getenv,
			})
			if err != nil {
				return err
			}
			return renderResult(env, res)
		},
	}
}
