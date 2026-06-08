package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/orafaelfragoso/columbus/internal/index"
	"github.com/orafaelfragoso/columbus/internal/project"
	"github.com/orafaelfragoso/columbus/internal/render"
)

// installResult is the one-shot onboarding summary: project identity plus the
// counts from the first index+embed pass.
type installResult struct {
	ProjectID          string   `json:"project_id"`
	ConfigPath         string   `json:"config_path"`
	DataDir            string   `json:"data_dir"`
	DBPath             string   `json:"db_path"`
	AlreadyInitialized bool     `json:"already_initialized"`
	Files              int      `json:"files"`
	Symbols            int      `json:"symbols"`
	Embedded           int      `json:"embedded"`
	Warnings           []string `json:"warnings,omitempty"`
}

func (installResult) CommandName() string { return "install" }

func (r installResult) RenderText(w io.Writer, _ render.Options) error {
	if r.AlreadyInitialized {
		fmt.Fprintf(w, "Reusing columbus project %s\n", r.ProjectID)
	} else {
		fmt.Fprintf(w, "Installed columbus project %s\n", r.ProjectID)
	}
	fmt.Fprintf(w, "  config:   %s\n", r.ConfigPath)
	fmt.Fprintf(w, "  data dir: %s\n", r.DataDir)
	fmt.Fprintf(w, "  indexed:  %d files, %d symbols, %d embedded\n", r.Files, r.Symbols, r.Embedded)
	for _, warn := range r.Warnings {
		fmt.Fprintf(w, "  warning:  %s\n", warn)
	}
	return nil
}

func (r installResult) RenderLLM(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "# columbus install\n\n- project_id: %s\n- files: %d\n- symbols: %d\n- embedded: %d\n",
		r.ProjectID, r.Files, r.Symbols, r.Embedded)
	return nil
}

func newInstallCmd(env *Env) *cobra.Command {
	var noEmbed bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Onboard a repository: write config, create the db, index and embed",
		Long: "Set up Columbus in the current directory in one step: write " +
			".columbus.json, create the project database, and run the first index " +
			"and embedding pass.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			init, err := project.Init(project.InitParams{
				WorkDir: env.WorkDir,
				IDs:     env.IDs,
				Getenv:  env.Getenv,
				Ctx:     env.ctx(),
			})
			if err != nil {
				return err
			}
			res, warnings, err := runIndex(env, index.ModeFull, noEmbed)
			if err != nil {
				return err
			}
			out := installResult{
				ProjectID:          init.ProjectID,
				ConfigPath:         init.ConfigPath,
				DataDir:            init.DataDir,
				DBPath:             init.DBPath,
				AlreadyInitialized: init.AlreadyInitialized,
				Files:              res.TotalFiles,
				Symbols:            res.Symbols,
				Embedded:           res.Embedded,
				Warnings:           append(init.Warnings, warnings...),
			}
			return renderResult(env, out)
		},
	}
	cmd.Flags().BoolVar(&noEmbed, "no-embed", false, "skip embeddings (metadata-only index)")
	return cmd
}
