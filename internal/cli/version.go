package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/rafaelfragoso/columbus/internal/contract"
	"github.com/rafaelfragoso/columbus/internal/render"
)

// VersionResult is the typed result of the version command.
type VersionResult struct {
	Version            string `json:"version"`
	Commit             string `json:"commit"`
	Date               string `json:"date"`
	IndexSchemaVersion int    `json:"index_schema_version"`
}

func (VersionResult) CommandName() string { return "version" }

func (r VersionResult) RenderText(w io.Writer, _ render.Options) error {
	_, err := fmt.Fprintf(w, "columbus %s (commit %s, built %s)\n", r.Version, r.Commit, r.Date)
	return err
}

func (r VersionResult) RenderLLM(w io.Writer, _ render.Options) error {
	_, err := fmt.Fprintf(w, "# columbus version\n\n- version: %s\n- commit: %s\n- date: %s\n- index schema: %d\n",
		r.Version, r.Commit, r.Date, r.IndexSchemaVersion)
	return err
}

func versionResult(env *Env) VersionResult {
	return VersionResult{
		Version:            valueOr(env.Version.Version, "dev"),
		Commit:             valueOr(env.Version.Commit, "none"),
		Date:               valueOr(env.Version.Date, "unknown"),
		IndexSchemaVersion: contract.SchemaVersion,
	}
}

func newVersionCmd(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return renderResult(env, versionResult(env))
		},
	}
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
