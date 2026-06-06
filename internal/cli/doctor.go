package cli

import (
	"github.com/spf13/cobra"

	"github.com/rafaelfragoso/columbus/internal/doctor"
)

func newDoctorCmd(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose the environment and project health",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, code := doctor.Run(doctor.Params{
				WorkDir: env.WorkDir,
				Getenv:  env.Getenv,
				Version: env.Version.Version,
				Ctx:     env.ctx(),
			})
			if err := renderResult(env, res); err != nil {
				return err
			}
			if code != "" {
				env.setExit(int(code.Exit()))
			}
			return nil
		},
	}
}
