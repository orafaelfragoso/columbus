package cli

import (
	"context"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/orafaelfragoso/columbus/internal/knowledge"
	"github.com/orafaelfragoso/columbus/internal/render"
)

// cmdContext is the (background) context used for slog.Logger.Log calls.
var cmdContext = context.Background()

// withKnowledge opens the project and invokes fn with a unified knowledge
// Manager, logging the outcome at the given level (mutations: info; reads:
// debug).
func withKnowledge(env *Env, cmdName string, level slog.Level, fn func(*knowledge.Manager) (render.Payload, error)) error {
	proj, err := env.openProject()
	if err != nil {
		return err
	}
	defer proj.Close()
	mgr := &knowledge.Manager{DB: proj.DB, Clock: env.Clock, WorkDir: env.WorkDir}
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
		Short: "Manage durable knowledge: epics, stories, tasks, context and tags",
		Long: "One surface over every durable-knowledge kind. <kind> is one of: " +
			strings.Join(knowledge.Kinds, ", ") + ".",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(
		newMemoryAddCmd(env),
		newMemoryUpdateCmd(env),
		newMemoryRemoveCmd(env),
		newMemoryListCmd(env),
	)
	return cmd
}

func newMemoryAddCmd(env *Env) *cobra.Command {
	var p knowledge.AddParams
	cmd := &cobra.Command{
		Use:   "add <kind>",
		Short: "Add a knowledge entity (epic|story|task|context)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withKnowledge(env, "memory.add", slog.LevelInfo, func(m *knowledge.Manager) (render.Payload, error) {
				return m.Add(args[0], p)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&p.Title, "title", "", "short title")
	f.StringVar(&p.Body, "body", "", "description / body")
	f.StringVar(&p.Parent, "parent", "", "parent id (epic for a story, story for a task)")
	f.StringVar(&p.Type, "type", "", "context sub-kind (decision|pattern|failure|command|glossary|note|reminder)")
	f.StringArrayVar(&p.Tags, "tag", nil, "tag (repeatable)")
	f.StringArrayVar(&p.Refs, "ref", nil, "context link file:<path>|symbol:<name> (repeatable)")
	f.StringArrayVar(&p.Evidence, "evidence", nil, "context evidence path:start-end (repeatable)")
	return cmd
}

func newMemoryUpdateCmd(env *Env) *cobra.Command {
	var title, body, parent, typ string
	var p knowledge.UpdateParams
	cmd := &cobra.Command{
		Use:   "update <kind> <id>",
		Short: "Update a knowledge entity (partial)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("title") {
				p.Title = &title
			}
			if cmd.Flags().Changed("body") {
				p.Body = &body
			}
			if cmd.Flags().Changed("parent") {
				p.Parent = &parent
			}
			if cmd.Flags().Changed("type") {
				p.Type = &typ
			}
			return withKnowledge(env, "memory.update", slog.LevelInfo, func(m *knowledge.Manager) (render.Payload, error) {
				return m.Update(args[0], args[1], p)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&title, "title", "", "new title")
	f.StringVar(&body, "body", "", "new body")
	f.StringVar(&parent, "parent", "", "re-parent (story→epic, task→story)")
	f.StringVar(&typ, "type", "", "re-type a context entry")
	f.StringVar(&p.Status, "status", "", "work kinds: record a status change ("+strings.Join(knowledge.Statuses(), "|")+")")
	f.StringVar(&p.Comment, "comment", "", "work kinds: append a note (with --status it annotates the change)")
	f.StringArrayVar(&p.AddTags, "add-tag", nil, "add tag (repeatable)")
	f.StringArrayVar(&p.RemoveTags, "remove-tag", nil, "remove tag (repeatable)")
	f.StringArrayVar(&p.AddRefs, "add-ref", nil, "add reference/link (repeatable)")
	f.StringArrayVar(&p.RemoveRefs, "remove-ref", nil, "remove reference/link (repeatable)")
	f.StringArrayVar(&p.AddEvidence, "add-evidence", nil, "context: add evidence path:start-end")
	f.StringArrayVar(&p.RemoveEvidence, "remove-evidence", nil, "context: remove evidence path:start-end")
	return cmd
}

func newMemoryRemoveCmd(env *Env) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "remove <kind> <id>",
		Short: "Remove a knowledge entity (hard delete; id retired)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withKnowledge(env, "memory.remove", slog.LevelInfo, func(m *knowledge.Manager) (render.Payload, error) {
				return m.Remove(args[0], args[1], force)
			})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "confirm destructive delete")
	return cmd
}

func newMemoryListCmd(env *Env) *cobra.Command {
	var f knowledge.ListFilter
	cmd := &cobra.Command{
		Use:   "list <kind>",
		Short: "List knowledge entities of a kind (epic|story|task|context|tag)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withKnowledge(env, "memory.list", slog.LevelDebug, func(m *knowledge.Manager) (render.Payload, error) {
				return m.List(args[0], f)
			})
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&f.Type, "type", "", "context sub-kind filter")
	fl.StringVar(&f.Parent, "parent", "", "filter work children by parent id")
	fl.StringVar(&f.Status, "status", "", "filter by status")
	fl.StringVar(&f.Tag, "tag", "", "filter by tag")
	return cmd
}
