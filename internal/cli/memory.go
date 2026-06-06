package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rafaelfragoso/columbus/internal/contract"
	"github.com/rafaelfragoso/columbus/internal/memory"
	"github.com/rafaelfragoso/columbus/internal/render"
)

// cmdContext is the (background) context used for slog.Logger.Log calls.
var cmdContext = context.Background()

// exportConfirm is a small payload confirming a file export.
type exportConfirm struct {
	Out   string `json:"out"`
	Count int    `json:"count"`
}

func (exportConfirm) CommandName() string { return "memory" }
func (c exportConfirm) RenderText(w io.Writer, _ render.Options) error {
	_, err := io.WriteString(w, "exported "+strconv.Itoa(c.Count)+" memorie(s) to "+c.Out+"\n")
	return err
}
func (c exportConfirm) RenderLLM(w io.Writer, _ render.Options) error {
	return c.RenderText(w, render.Options{})
}

func newMemoryCmd(env *Env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Manage the project's durable memory",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		newMemoryAddCmd(env),
		newMemoryEditCmd(env),
		newMemoryRemoveCmd(env),
		newMemoryLinkCmd(env),
		newMemoryListCmd(env),
		newMemorySearchCmd(env),
		newMemoryValidateCmd(env),
		newMemoryExportCmd(env),
		newMemoryImportCmd(env),
	)
	return cmd
}

// withManager opens the project and invokes fn with a memory Manager, logging
// the outcome at the given level (mutations: info; reads: debug).
func withManager(env *Env, cmdName string, level slog.Level, fn func(*memory.Manager) (render.Payload, error)) error {
	proj, err := env.openProject()
	if err != nil {
		return err
	}
	defer proj.Close()
	mgr := &memory.Manager{DB: proj.DB, Clock: env.Clock, WorkDir: env.WorkDir}
	payload, err := fn(mgr)
	if err != nil {
		proj.Logger.Info(cmdName+" failed", "error", err.Error())
		return err
	}
	proj.Logger.Log(cmdContext, level, cmdName)
	return renderResult(env, payload)
}

func parseEvidence(specs []string) ([]memory.EvidenceSpec, error) {
	var out []memory.EvidenceSpec
	for _, s := range specs {
		ev, err := memory.ParseEvidence(s)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
}

func parseLinks(specs []string) ([]memory.LinkSpec, error) {
	var out []memory.LinkSpec
	for _, s := range specs {
		l, err := memory.ParseLink(s)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

func newMemoryAddCmd(env *Env) *cobra.Command {
	var kind, title, body string
	var evidence, links, tags []string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a memory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ev, err := parseEvidence(evidence)
			if err != nil {
				return err
			}
			lk, err := parseLinks(links)
			if err != nil {
				return err
			}
			return withManager(env, "memory.add", slog.LevelInfo, func(m *memory.Manager) (render.Payload, error) {
				return m.Add(memory.AddParams{Kind: kind, Title: title, Body: body, Evidence: ev, Links: lk, Tags: tags})
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&kind, "kind", "", "memory kind: "+strings.Join(memory.Kinds, "|"))
	f.StringVar(&title, "title", "", "short title")
	f.StringVar(&body, "body", "", "memory body")
	f.StringArrayVar(&evidence, "evidence", nil, "evidence anchor path:start-end (repeatable)")
	f.StringArrayVar(&links, "link", nil, "link file:<path> or symbol:<name> (repeatable)")
	f.StringArrayVar(&tags, "tag", nil, "tag (repeatable)")
	return cmd
}

func newMemoryEditCmd(env *Env) *cobra.Command {
	var title, body, kind string
	var addTags, removeTags, addEvidence, removeEvidence, addLinks, removeLinks []string
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit a memory (partial)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := memory.EditParams{AddTags: addTags, RemoveTags: removeTags}
			if cmd.Flags().Changed("title") {
				p.Title = &title
			}
			if cmd.Flags().Changed("body") {
				p.Body = &body
			}
			if cmd.Flags().Changed("kind") {
				p.Kind = &kind
			}
			var err error
			if p.AddEvidence, err = parseEvidence(addEvidence); err != nil {
				return err
			}
			if p.RemoveEvidence, err = parseEvidence(removeEvidence); err != nil {
				return err
			}
			if p.AddLinks, err = parseLinks(addLinks); err != nil {
				return err
			}
			if p.RemoveLinks, err = parseLinks(removeLinks); err != nil {
				return err
			}
			return withManager(env, "memory.edit", slog.LevelInfo, func(m *memory.Manager) (render.Payload, error) {
				return m.Edit(args[0], p)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&title, "title", "", "new title")
	f.StringVar(&body, "body", "", "new body")
	f.StringVar(&kind, "kind", "", "new kind")
	f.StringArrayVar(&addTags, "add-tag", nil, "add tag (repeatable)")
	f.StringArrayVar(&removeTags, "remove-tag", nil, "remove tag (repeatable)")
	f.StringArrayVar(&addEvidence, "add-evidence", nil, "add evidence path:start-end")
	f.StringArrayVar(&removeEvidence, "remove-evidence", nil, "remove evidence path:start-end")
	f.StringArrayVar(&addLinks, "add-link", nil, "add link")
	f.StringArrayVar(&removeLinks, "remove-link", nil, "remove link")
	return cmd
}

func newMemoryRemoveCmd(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a memory (hard delete; id retired)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withManager(env, "memory.remove", slog.LevelInfo, func(m *memory.Manager) (render.Payload, error) {
				return m.Remove(args[0])
			})
		},
	}
}

func newMemoryLinkCmd(env *Env) *cobra.Command {
	var links []string
	cmd := &cobra.Command{
		Use:   "link <id>",
		Short: "Add links to a memory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lk, err := parseLinks(links)
			if err != nil {
				return err
			}
			return withManager(env, "memory.link", slog.LevelInfo, func(m *memory.Manager) (render.Payload, error) {
				return m.Link(args[0], lk)
			})
		},
	}
	cmd.Flags().StringArrayVar(&links, "link", nil, "link file:<path> or symbol:<name> (repeatable)")
	return cmd
}

func newMemoryListCmd(env *Env) *cobra.Command {
	var kind, tag string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List memories",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withManager(env, "memory.list", slog.LevelDebug, func(m *memory.Manager) (render.Payload, error) {
				return m.List(kind, tag)
			})
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "filter by kind")
	cmd.Flags().StringVar(&tag, "tag", "", "filter by tag")
	return cmd
}

func newMemorySearchCmd(env *Env) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search memories (FTS5)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withManager(env, "memory.search", slog.LevelDebug, func(m *memory.Manager) (render.Payload, error) {
				return m.Search(strings.Join(args, " "), limit)
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum results")
	return cmd
}

func newMemoryValidateCmd(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate memory evidence and links (drift is a warning)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withManager(env, "memory.validate", slog.LevelDebug, func(m *memory.Manager) (render.Payload, error) {
				return m.Validate()
			})
		},
	}
}

func newMemoryExportCmd(env *Env) *cobra.Command {
	var kind, tag, out string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export memories as a portable JSON document (stdout by default)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			proj, err := env.openProject()
			if err != nil {
				return err
			}
			defer proj.Close()
			mgr := &memory.Manager{DB: proj.DB, Clock: env.Clock, WorkDir: env.WorkDir}
			doc, err := mgr.Export(kind, tag)
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
	cmd.Flags().StringVar(&kind, "kind", "", "filter by kind")
	cmd.Flags().StringVar(&tag, "tag", "", "filter by tag")
	cmd.Flags().StringVar(&out, "out", "", "write to a file instead of stdout")
	return cmd
}

func newMemoryImportCmd(env *Env) *cobra.Command {
	var preserveIDs bool
	cmd := &cobra.Command{
		Use:   "import [path]",
		Short: "Import memories from a JSON document (path or stdin)",
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
			return withManager(env, "memory.import", slog.LevelInfo, func(m *memory.Manager) (render.Payload, error) {
				return m.Import(doc, preserveIDs)
			})
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
	stdin, ok := env.Stdin.(io.Reader)
	if !ok || stdin == nil {
		return nil, contract.Errorf(contract.CodeUsage, "no import file given and no stdin available")
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return nil, &contract.Error{Code: contract.CodeStoreError, Message: err.Error()}
	}
	return raw, nil
}
