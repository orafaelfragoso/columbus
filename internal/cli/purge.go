package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/orafaelfragoso/columbus/internal/config"
	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/render"
	"github.com/orafaelfragoso/columbus/internal/store"
)

// purgeResult reports a completed purge.
type purgeResult struct {
	ProjectID string `json:"project_id"`
	DBPath    string `json:"db_path"`
	Purged    bool   `json:"purged"`
}

func (purgeResult) CommandName() string { return "purge" }
func (r purgeResult) RenderText(w io.Writer, _ render.Options) error {
	fmt.Fprintf(w, "purged all data for project %s\n  fresh db:  %s\n  config reset to defaults\n", r.ProjectID, r.DBPath)
	return nil
}
func (r purgeResult) RenderLLM(w io.Writer, o render.Options) error { return r.RenderText(w, o) }

func newPurgeCmd(env *Env) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "purge",
		Short: "Clear all records and reset config to defaults (keeps the project)",
		Long:  "Drop every record from the project database and reset .columbus.json to defaults. Files stay; data is gone.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			loc, err := env.projectLocation()
			if err != nil {
				return err
			}
			what := fmt.Sprintf("About to erase all records in %s and reset config to defaults", loc.Paths.DBPath)
			if err := confirmDestructive(env, yes, what); err != nil {
				return err
			}

			// Drop the database file (and WAL/SHM sidecars), then recreate an empty
			// schema and re-stamp the project id.
			for _, p := range []string{loc.Paths.DBPath, loc.Paths.DBPath + "-wal", loc.Paths.DBPath + "-shm"} {
				if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
					return &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
				}
			}
			db, err := store.Open(loc.Paths.DBPath)
			if err != nil {
				return err
			}
			if err := db.Meta().SetProjectID(loc.ProjectID); err != nil {
				db.Close()
				return err
			}
			db.Close()

			cfg := config.Default()
			cfg.ProjectID = loc.ProjectID
			if err := config.Save(loc.ConfigPath, cfg); err != nil {
				return err
			}
			return renderResult(env, purgeResult{ProjectID: loc.ProjectID, DBPath: loc.Paths.DBPath, Purged: true})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm without prompting (required when not a TTY)")
	return cmd
}
