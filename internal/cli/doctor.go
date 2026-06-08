package cli

import (
	"github.com/spf13/cobra"

	"github.com/orafaelfragoso/columbus/internal/doctor"
	"github.com/orafaelfragoso/columbus/internal/embed"
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
				Embed:   embedProbe(env),
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

// embedProbe returns a doctor runtime probe: it loads the embedding runtime
// (onnxruntime + embedded model) and reports the model name and dimension,
// closing the engine immediately. It keeps the doctor package free of the
// onnxruntime dependency.
func embedProbe(env *Env) func() (string, int, error) {
	return func() (string, int, error) {
		e, err := embed.New(env.ctx())
		if err != nil {
			return "", 0, err
		}
		defer e.Close()
		return e.Model(), e.Dim(), nil
	}
}
