package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/render"
)

// uninstallResult reports what uninstall removed.
type uninstallResult struct {
	ProjectID  string `json:"project_id"`
	ConfigPath string `json:"config_path"`
	ProjectDir string `json:"project_dir"`
	Removed    bool   `json:"removed"`
}

func (uninstallResult) CommandName() string { return "uninstall" }
func (r uninstallResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "uninstalled columbus project %s\n  removed config: %s\n  removed data:   %s\n",
		r.ProjectID, r.ConfigPath, r.ProjectDir)
	return nil
}
func (r uninstallResult) RenderLLM(w io.Writer, o render.Options) error { return r.RenderText(w, o) }

func newUninstallCmd(env *Env) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove Columbus from this project: delete config and the project database",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			loc, err := env.projectLocation()
			if err != nil {
				return err
			}
			what := fmt.Sprintf("About to delete %s and all project data at %s", loc.ConfigPath, loc.Paths.ProjectDir)
			if err := confirmDestructive(env, yes, what); err != nil {
				return err
			}
			if err := os.RemoveAll(loc.Paths.ProjectDir); err != nil {
				return &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
			}
			if err := os.Remove(loc.ConfigPath); err != nil && !os.IsNotExist(err) {
				return &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
			}
			return renderResult(env, uninstallResult{
				ProjectID: loc.ProjectID, ConfigPath: loc.ConfigPath,
				ProjectDir: loc.Paths.ProjectDir, Removed: true,
			})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm without prompting (required when not a TTY)")
	return cmd
}
