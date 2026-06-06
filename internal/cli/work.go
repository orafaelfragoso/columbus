package cli

import (
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rafaelfragoso/columbus/internal/render"
	"github.com/rafaelfragoso/columbus/internal/work"
)

// withWorkManager opens the project and invokes fn with a work Manager, logging
// the outcome at the given level (mutations: info; reads: debug).
func withWorkManager(env *Env, cmdName string, level slog.Level, fn func(*work.Manager) (render.Payload, error)) error {
	proj, err := env.openProject()
	if err != nil {
		return err
	}
	defer proj.Close()
	mgr := &work.Manager{DB: proj.DB, Clock: env.Clock}
	payload, err := fn(mgr)
	if err != nil {
		proj.Logger.Info(cmdName+" failed", "error", err.Error())
		return err
	}
	proj.Logger.Log(cmdContext, level, cmdName)
	return renderResult(env, payload)
}

func newEpicCmd(env *Env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "epic",
		Short: "Manage epics (structured memory)",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		newEpicAddCmd(env),
		newEpicEditCmd(env),
		newEpicDeleteCmd(env),
		newEpicListCmd(env),
		newEpicStatusCmd(env),
		newEpicCommentCmd(env),
		newEpicRefCmd(env),
		newEpicValidateCmd(env),
	)
	return cmd
}

func newTaskCmd(env *Env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Manage tasks under epics (structured memory)",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		newTaskAddCmd(env),
		newTaskEditCmd(env),
		newTaskDeleteCmd(env),
		newTaskListCmd(env),
		newTaskStatusCmd(env),
		newTaskCommentCmd(env),
		newTaskRefCmd(env),
		newTaskValidateCmd(env),
	)
	return cmd
}

// refFlags binds the shared --file/--dir/--memory/--symbol/--remove-ref flags
// and assembles the add/remove RefSpec lists at run time.
type refFlags struct {
	files, dirs, memories, symbols, removes []string
}

func (rf *refFlags) bind(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringArrayVar(&rf.files, "file", nil, "reference an indexed file path (repeatable)")
	f.StringArrayVar(&rf.dirs, "dir", nil, "reference a directory prefix (repeatable)")
	f.StringArrayVar(&rf.memories, "memory", nil, "reference a memory id mem_NNN (repeatable)")
	f.StringArrayVar(&rf.symbols, "symbol", nil, "reference a symbol name (repeatable)")
	f.StringArrayVar(&rf.removes, "remove-ref", nil, "remove a ref by <type>:<ref> (repeatable)")
}

func (rf *refFlags) specs() (add, remove []work.RefSpec, err error) {
	for _, v := range rf.files {
		add = append(add, work.RefSpec{Type: "file", Ref: v})
	}
	for _, v := range rf.dirs {
		add = append(add, work.RefSpec{Type: "dir", Ref: v})
	}
	for _, v := range rf.memories {
		add = append(add, work.RefSpec{Type: "memory", Ref: v})
	}
	for _, v := range rf.symbols {
		add = append(add, work.RefSpec{Type: "symbol", Ref: v})
	}
	for _, v := range rf.removes {
		spec, perr := work.ParseRef(v)
		if perr != nil {
			return nil, nil, perr
		}
		remove = append(remove, spec)
	}
	return add, remove, nil
}

func newEpicRefCmd(env *Env) *cobra.Command {
	rf := &refFlags{}
	cmd := &cobra.Command{
		Use:   "ref <id>",
		Short: "Add or remove epic references (drift is a warning)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			add, remove, err := rf.specs()
			if err != nil {
				return err
			}
			return withWorkManager(env, "epic.ref", slog.LevelInfo, func(m *work.Manager) (render.Payload, error) {
				return m.EpicRef(args[0], add, remove)
			})
		},
	}
	rf.bind(cmd)
	return cmd
}

func newEpicValidateCmd(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Scan epic references for drift (warnings)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withWorkManager(env, "epic.validate", slog.LevelDebug, func(m *work.Manager) (render.Payload, error) {
				return m.EpicValidate()
			})
		},
	}
}

func newTaskRefCmd(env *Env) *cobra.Command {
	rf := &refFlags{}
	cmd := &cobra.Command{
		Use:   "ref <id>",
		Short: "Add or remove task references (drift is a warning)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			add, remove, err := rf.specs()
			if err != nil {
				return err
			}
			return withWorkManager(env, "task.ref", slog.LevelInfo, func(m *work.Manager) (render.Payload, error) {
				return m.TaskRef(args[0], add, remove)
			})
		},
	}
	rf.bind(cmd)
	return cmd
}

func newTaskValidateCmd(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Scan task references for drift (warnings)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withWorkManager(env, "task.validate", slog.LevelDebug, func(m *work.Manager) (render.Payload, error) {
				return m.TaskValidate()
			})
		},
	}
}

func newEpicAddCmd(env *Env) *cobra.Command {
	var title, body string
	var tags []string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add an epic",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withWorkManager(env, "epic.add", slog.LevelInfo, func(m *work.Manager) (render.Payload, error) {
				return m.EpicAdd(work.EpicAddParams{Title: title, Body: body, Tags: tags})
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&title, "title", "", "short title")
	f.StringVar(&body, "body", "", "description")
	f.StringArrayVar(&tags, "tag", nil, "tag (repeatable)")
	return cmd
}

func newEpicEditCmd(env *Env) *cobra.Command {
	var title, body string
	var addTags, removeTags []string
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit an epic (partial; metadata only, no event)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := work.EpicEditParams{AddTags: addTags, RemoveTags: removeTags}
			if cmd.Flags().Changed("title") {
				p.Title = &title
			}
			if cmd.Flags().Changed("body") {
				p.Body = &body
			}
			return withWorkManager(env, "epic.edit", slog.LevelInfo, func(m *work.Manager) (render.Payload, error) {
				return m.EpicEdit(args[0], p)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&title, "title", "", "new title")
	f.StringVar(&body, "body", "", "new body")
	f.StringArrayVar(&addTags, "add-tag", nil, "add tag (repeatable)")
	f.StringArrayVar(&removeTags, "remove-tag", nil, "remove tag (repeatable)")
	return cmd
}

func newEpicDeleteCmd(env *Env) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an epic (hard; cascades tasks; id retired)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withWorkManager(env, "epic.delete", slog.LevelInfo, func(m *work.Manager) (render.Payload, error) {
				return m.EpicDelete(args[0], force)
			})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "confirm destructive delete")
	return cmd
}

func newEpicListCmd(env *Env) *cobra.Command {
	var status, tag string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List epics",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withWorkManager(env, "epic.list", slog.LevelDebug, func(m *work.Manager) (render.Payload, error) {
				return m.EpicList(status, tag)
			})
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by status")
	cmd.Flags().StringVar(&tag, "tag", "", "filter by tag")
	return cmd
}

func newEpicStatusCmd(env *Env) *cobra.Command {
	var to, comment string
	cmd := &cobra.Command{
		Use:   "status <id>",
		Short: "Record an epic status change (appends an event)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withWorkManager(env, "epic.status", slog.LevelInfo, func(m *work.Manager) (render.Payload, error) {
				return m.EpicStatus(args[0], to, comment)
			})
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "status: "+strings.Join(work.Statuses, "|"))
	cmd.Flags().StringVar(&comment, "comment", "", "optional note recorded with the change")
	return cmd
}

func newEpicCommentCmd(env *Env) *cobra.Command {
	var text string
	cmd := &cobra.Command{
		Use:   "comment <id>",
		Short: "Append a progress note to an epic",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withWorkManager(env, "epic.comment", slog.LevelInfo, func(m *work.Manager) (render.Payload, error) {
				return m.EpicComment(args[0], text)
			})
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "note text")
	return cmd
}

func newTaskAddCmd(env *Env) *cobra.Command {
	var epic, title, body string
	var tags []string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a task under an epic",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withWorkManager(env, "task.add", slog.LevelInfo, func(m *work.Manager) (render.Payload, error) {
				return m.TaskAdd(work.TaskAddParams{Epic: epic, Title: title, Body: body, Tags: tags})
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

func newTaskEditCmd(env *Env) *cobra.Command {
	var title, body, epic string
	var addTags, removeTags []string
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit a task (partial; --epic re-parents)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := work.TaskEditParams{AddTags: addTags, RemoveTags: removeTags}
			if cmd.Flags().Changed("title") {
				p.Title = &title
			}
			if cmd.Flags().Changed("body") {
				p.Body = &body
			}
			if cmd.Flags().Changed("epic") {
				p.Epic = &epic
			}
			return withWorkManager(env, "task.edit", slog.LevelInfo, func(m *work.Manager) (render.Payload, error) {
				return m.TaskEdit(args[0], p)
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

func newTaskDeleteCmd(env *Env) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a task (hard; id retired)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withWorkManager(env, "task.delete", slog.LevelInfo, func(m *work.Manager) (render.Payload, error) {
				return m.TaskDelete(args[0], force)
			})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "confirm destructive delete")
	return cmd
}

func newTaskListCmd(env *Env) *cobra.Command {
	var epic, status, tag string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withWorkManager(env, "task.list", slog.LevelDebug, func(m *work.Manager) (render.Payload, error) {
				return m.TaskList(epic, status, tag)
			})
		},
	}
	cmd.Flags().StringVar(&epic, "epic", "", "filter by parent epic id")
	cmd.Flags().StringVar(&status, "status", "", "filter by status")
	cmd.Flags().StringVar(&tag, "tag", "", "filter by tag")
	return cmd
}

func newTaskStatusCmd(env *Env) *cobra.Command {
	var to, comment string
	cmd := &cobra.Command{
		Use:   "status <id>",
		Short: "Record a task status change (appends an event)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withWorkManager(env, "task.status", slog.LevelInfo, func(m *work.Manager) (render.Payload, error) {
				return m.TaskStatus(args[0], to, comment)
			})
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "status: "+strings.Join(work.Statuses, "|"))
	cmd.Flags().StringVar(&comment, "comment", "", "optional note recorded with the change")
	return cmd
}

func newTaskCommentCmd(env *Env) *cobra.Command {
	var text string
	cmd := &cobra.Command{
		Use:   "comment <id>",
		Short: "Append a progress note to a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withWorkManager(env, "task.comment", slog.LevelInfo, func(m *work.Manager) (render.Payload, error) {
				return m.TaskComment(args[0], text)
			})
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "note text")
	return cmd
}
