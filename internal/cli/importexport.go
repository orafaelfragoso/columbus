package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/orafaelfragoso/columbus/internal/contract"
	"github.com/orafaelfragoso/columbus/internal/memory"
	"github.com/orafaelfragoso/columbus/internal/render"
)

// exportConfirm is a small payload confirming a file export.
type exportConfirm struct {
	Out   string `json:"out"`
	Count int    `json:"count"`
}

func (exportConfirm) CommandName() string { return "export" }
func (c exportConfirm) RenderText(w io.Writer, _ render.Options) error {
	_, err := io.WriteString(w, "exported "+strconv.Itoa(c.Count)+" memories to "+c.Out+"\n")
	return err
}
func (c exportConfirm) RenderLLM(w io.Writer, o render.Options) error { return c.RenderText(w, o) }

func newExportCmd(env *Env) *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export all project memories (with tags, links, evidence) as JSON",
		Long: "Export the durable memory layer as a portable JSON document " +
			"(stdout by default). Vectors are not exported; reindex rebuilds them.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			proj, err := env.openProject()
			if err != nil {
				return err
			}
			defer proj.Close()
			mgr := &memory.Manager{DB: proj.DB, Clock: env.Clock, WorkDir: env.WorkDir}
			doc, err := mgr.Export("", "")
			if err != nil {
				return err
			}

			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			enc.SetIndent("", "  ")
			if err := enc.Encode(doc); err != nil {
				return &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
			}
			if out == "" {
				_, werr := env.Stdout.Write(buf.Bytes())
				return werr
			}
			if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
				return &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
			}
			return renderResult(env, exportConfirm{Out: out, Count: len(doc.Memories)})
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "write to a file instead of stdout")
	return cmd
}

func newImportCmd(env *Env) *cobra.Command {
	var preserveIDs bool
	cmd := &cobra.Command{
		Use:   "import [path]",
		Short: "Import project knowledge from a JSON document (path or stdin)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := readImportInput(env, args)
			if err != nil {
				return err
			}
			var doc memory.ExportDoc
			if err := json.Unmarshal(raw, &doc); err != nil {
				return &contract.Error{Code: contract.CodeConfigInvalid, Message: "invalid import document: " + err.Error()}
			}
			proj, err := env.openProject()
			if err != nil {
				return err
			}
			defer proj.Close()
			mgr := &memory.Manager{DB: proj.DB, Clock: env.Clock, WorkDir: env.WorkDir}
			res, err := mgr.Import(doc, preserveIDs)
			if err != nil {
				proj.Logger.Info("import failed", "error", err.Error())
				return err
			}
			proj.Logger.Log(cmdContext, slog.LevelInfo, "import")
			return renderResult(env, res)
		},
	}
	cmd.Flags().BoolVar(&preserveIDs, "preserve-ids", false, "restore original ids (errors on collision)")
	return cmd
}

func readImportInput(env *Env, args []string) ([]byte, error) {
	if len(args) == 1 {
		raw, err := os.ReadFile(args[0])
		if err != nil {
			return nil, &contract.Error{Code: contract.CodeNotFound, Message: "cannot read import file: " + err.Error()}
		}
		return raw, nil
	}
	if env.Stdin == nil {
		return nil, contract.Errorf(contract.CodeUsage, "no import file given and no stdin available")
	}
	raw, err := io.ReadAll(env.Stdin)
	if err != nil {
		return nil, &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}
	return raw, nil
}
