package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/rafaelfragoso/columbus/internal/memory"
	"github.com/rafaelfragoso/columbus/internal/render"
)

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
	)
	return cmd
}

// withManager opens the project and invokes fn with a memory Manager.
func withManager(env *Env, fn func(*memory.Manager) (render.Payload, error)) error {
	proj, err := env.openProject()
	if err != nil {
		return err
	}
	defer proj.DB.Close()
	mgr := &memory.Manager{DB: proj.DB, Clock: env.Clock, WorkDir: env.WorkDir}
	payload, err := fn(mgr)
	if err != nil {
		return err
	}
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
			return withManager(env, func(m *memory.Manager) (render.Payload, error) {
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
			return withManager(env, func(m *memory.Manager) (render.Payload, error) {
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
			return withManager(env, func(m *memory.Manager) (render.Payload, error) {
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
			return withManager(env, func(m *memory.Manager) (render.Payload, error) {
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
			return withManager(env, func(m *memory.Manager) (render.Payload, error) {
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
			return withManager(env, func(m *memory.Manager) (render.Payload, error) {
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
			return withManager(env, func(m *memory.Manager) (render.Payload, error) {
				return m.Validate()
			})
		},
	}
}
