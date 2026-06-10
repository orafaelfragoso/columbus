package cli

import (
	"context"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/orafaelfragoso/columbus/internal/memory"
	"github.com/orafaelfragoso/columbus/internal/render"
)

// cmdContext is the (background) context used for slog.Logger.Log calls.
var cmdContext = context.Background()

// withMemory opens the project and invokes fn with a memory Manager, logging
// the outcome at the given level (mutations: info; reads: debug).
func withMemory(env *Env, cmdName string, level slog.Level, fn func(*memory.Manager) (render.Payload, error)) error {
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

func newMemoryCmd(env *Env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Manage durable project memory (adr, plan, documentation)",
		Long: "The project's durable record. <kind> is one of: " +
			strings.Join(memory.Kinds, ", ") + ".",
		Args: cobra.NoArgs,
		// RunE makes unknown subcommands a usage error (exit 2) instead of
		// silently printing help with exit 0.
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newMemoryAddCmd(env),
		newMemoryUpdateCmd(env),
		newMemoryRemoveCmd(env),
		newMemoryListCmd(env),
		newMemoryValidateCmd(env),
	)
	return cmd
}

// parseSpecs parses repeated --link and --evidence flag values.
func parseSpecs(links, evidence []string) ([]memory.LinkSpec, []memory.EvidenceSpec, error) {
	var ls []memory.LinkSpec
	for _, s := range links {
		l, err := memory.ParseLink(s)
		if err != nil {
			return nil, nil, err
		}
		ls = append(ls, l)
	}
	var evs []memory.EvidenceSpec
	for _, s := range evidence {
		ev, err := memory.ParseEvidence(s)
		if err != nil {
			return nil, nil, err
		}
		evs = append(evs, ev)
	}
	return ls, evs, nil
}

func newMemoryAddCmd(env *Env) *cobra.Command {
	var title, body string
	var tags, links, evidence []string
	cmd := &cobra.Command{
		Use:   "add <kind>",
		Short: "Add a memory (" + strings.Join(memory.Kinds, "|") + ")",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ls, evs, err := parseSpecs(links, evidence)
			if err != nil {
				return err
			}
			return withMemory(env, "memory.add", slog.LevelInfo, func(m *memory.Manager) (render.Payload, error) {
				return m.Add(memory.AddParams{
					Kind: args[0], Title: title, Body: body,
					Tags: tags, Links: ls, Evidence: evs,
				})
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&title, "title", "", "short title")
	f.StringVar(&body, "body", "", "description / body")
	f.StringArrayVar(&tags, "tag", nil, "tag (repeatable)")
	f.StringArrayVar(&links, "link", nil, "link file:<path>|symbol:<name> (repeatable)")
	f.StringArrayVar(&evidence, "evidence", nil, "evidence path:start-end (repeatable)")
	return cmd
}

func newMemoryUpdateCmd(env *Env) *cobra.Command {
	var title, body, kind string
	var addTags, removeTags, addLinks, removeLinks, addEvidence, removeEvidence []string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a memory (partial)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var p memory.EditParams
			if cmd.Flags().Changed("title") {
				p.Title = &title
			}
			if cmd.Flags().Changed("body") {
				p.Body = &body
			}
			if cmd.Flags().Changed("kind") {
				p.Kind = &kind
			}
			p.AddTags, p.RemoveTags = addTags, removeTags
			var err error
			if p.AddLinks, p.AddEvidence, err = parseSpecs(addLinks, addEvidence); err != nil {
				return err
			}
			if p.RemoveLinks, p.RemoveEvidence, err = parseSpecs(removeLinks, removeEvidence); err != nil {
				return err
			}
			return withMemory(env, "memory.update", slog.LevelInfo, func(m *memory.Manager) (render.Payload, error) {
				return m.Edit(args[0], p)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&title, "title", "", "new title")
	f.StringVar(&body, "body", "", "new body")
	f.StringVar(&kind, "kind", "", "re-kind ("+strings.Join(memory.Kinds, "|")+")")
	f.StringArrayVar(&addTags, "add-tag", nil, "add tag (repeatable)")
	f.StringArrayVar(&removeTags, "remove-tag", nil, "remove tag (repeatable)")
	f.StringArrayVar(&addLinks, "add-link", nil, "add link file:<path>|symbol:<name> (repeatable)")
	f.StringArrayVar(&removeLinks, "remove-link", nil, "remove link (repeatable)")
	f.StringArrayVar(&addEvidence, "add-evidence", nil, "add evidence path:start-end (repeatable)")
	f.StringArrayVar(&removeEvidence, "remove-evidence", nil, "remove evidence path:start-end (repeatable)")
	return cmd
}

func newMemoryRemoveCmd(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a memory (hard delete; id retired)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMemory(env, "memory.remove", slog.LevelInfo, func(m *memory.Manager) (render.Payload, error) {
				return m.Remove(args[0])
			})
		},
	}
}

func newMemoryValidateCmd(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate memories: evidence drift and link resolution",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMemory(env, "memory.validate", slog.LevelDebug, func(m *memory.Manager) (render.Payload, error) {
				return m.Validate()
			})
		},
	}
}

func newMemoryListCmd(env *Env) *cobra.Command {
	var kind, tag string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List memories, optionally filtered by --kind and --tag",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withMemory(env, "memory.list", slog.LevelDebug, func(m *memory.Manager) (render.Payload, error) {
				return m.List(kind, tag)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&kind, "kind", "", "filter by kind ("+strings.Join(memory.Kinds, "|")+")")
	f.StringVar(&tag, "tag", "", "filter by tag")
	return cmd
}
