package cli

import (
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/orafaelfragoso/columbus/internal/render"
	"github.com/orafaelfragoso/columbus/internal/work"
)

func newStoryCmd(env *Env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "story",
		Short: "Manage stories under epics (structured memory)",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		newStoryAddCmd(env),
		newStoryEditCmd(env),
		newStoryDeleteCmd(env),
		newStoryListCmd(env),
		newStoryStatusCmd(env),
		newStoryCommentCmd(env),
		newStoryRefCmd(env),
		newStoryValidateCmd(env),
	)
	return cmd
}

func newStoryAddCmd(env *Env) *cobra.Command {
	var epic, title, body string
	var tags []string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a story under an epic",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withWorkManager(env, "story.add", slog.LevelInfo, func(m *work.Manager) (render.Payload, error) {
				return m.StoryAdd(work.StoryAddParams{Epic: epic, Title: title, Body: body, Tags: tags})
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&epic, "epic", "", "parent epic id (epic_NNN)")
	f.StringVar(&title, "title", "", "short title")
	f.StringVar(&body, "body", "", "description")
	f.StringArrayVar(&tags, "tag", nil, "tag (repeatable)")
	return cmd
}

func newStoryEditCmd(env *Env) *cobra.Command {
	var title, body, epic string
	var addTags, removeTags []string
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit a story (partial; --epic re-parents)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := work.StoryEditParams{AddTags: addTags, RemoveTags: removeTags}
			if cmd.Flags().Changed("title") {
				p.Title = &title
			}
			if cmd.Flags().Changed("body") {
				p.Body = &body
			}
			if cmd.Flags().Changed("epic") {
				p.Epic = &epic
			}
			return withWorkManager(env, "story.edit", slog.LevelInfo, func(m *work.Manager) (render.Payload, error) {
				return m.StoryEdit(args[0], p)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&title, "title", "", "new title")
	f.StringVar(&body, "body", "", "new body")
	f.StringVar(&epic, "epic", "", "re-parent to this epic id")
	f.StringArrayVar(&addTags, "add-tag", nil, "add tag (repeatable)")
	f.StringArrayVar(&removeTags, "remove-tag", nil, "remove tag (repeatable)")
	return cmd
}

func newStoryDeleteCmd(env *Env) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a story (hard; cascades tasks; id retired)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withWorkManager(env, "story.delete", slog.LevelInfo, func(m *work.Manager) (render.Payload, error) {
				return m.StoryDelete(args[0], force)
			})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "confirm destructive delete")
	return cmd
}

func newStoryListCmd(env *Env) *cobra.Command {
	var epic, status, tag string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List stories",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withWorkManager(env, "story.list", slog.LevelDebug, func(m *work.Manager) (render.Payload, error) {
				return m.StoryList(epic, status, tag)
			})
		},
	}
	cmd.Flags().StringVar(&epic, "epic", "", "filter by parent epic id")
	cmd.Flags().StringVar(&status, "status", "", "filter by status")
	cmd.Flags().StringVar(&tag, "tag", "", "filter by tag")
	return cmd
}

func newStoryStatusCmd(env *Env) *cobra.Command {
	var to, comment string
	cmd := &cobra.Command{
		Use:   "status <id>",
		Short: "Record a story status change (appends an event)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withWorkManager(env, "story.status", slog.LevelInfo, func(m *work.Manager) (render.Payload, error) {
				return m.StoryStatus(args[0], to, comment)
			})
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "status: "+strings.Join(work.Statuses, "|"))
	cmd.Flags().StringVar(&comment, "comment", "", "optional note recorded with the change")
	return cmd
}

func newStoryCommentCmd(env *Env) *cobra.Command {
	var text string
	cmd := &cobra.Command{
		Use:   "comment <id>",
		Short: "Append a progress note to a story",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withWorkManager(env, "story.comment", slog.LevelInfo, func(m *work.Manager) (render.Payload, error) {
				return m.StoryComment(args[0], text)
			})
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "note text")
	return cmd
}

func newStoryRefCmd(env *Env) *cobra.Command {
	rf := &refFlags{}
	cmd := &cobra.Command{
		Use:   "ref <id>",
		Short: "Add or remove story references (drift is a warning)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			add, remove, err := rf.specs()
			if err != nil {
				return err
			}
			return withWorkManager(env, "story.ref", slog.LevelInfo, func(m *work.Manager) (render.Payload, error) {
				return m.StoryRef(args[0], add, remove)
			})
		},
	}
	rf.bind(cmd)
	return cmd
}

func newStoryValidateCmd(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Scan story references for drift (warnings)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withWorkManager(env, "story.validate", slog.LevelDebug, func(m *work.Manager) (render.Payload, error) {
				return m.StoryValidate()
			})
		},
	}
}
