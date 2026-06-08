package cli

import (
	"github.com/spf13/cobra"

	"github.com/orafaelfragoso/columbus/internal/render"
	"github.com/orafaelfragoso/columbus/internal/show"
)

// newGraphsCmd projects the indexed file dependency graph (nodes + edges). It
// replaces the former `show graph` subcommand. The global --depth flag bounds
// traversal (0 = direct edges only).
func newGraphsCmd(env *Env) *cobra.Command {
	var in, role, lang string
	var max int
	cmd := &cobra.Command{
		Use:   "graphs",
		Short: "Project the indexed file dependency graph (nodes + edges)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withShower(env, "graphs", func(s *show.Shower) (render.Payload, error) {
				return s.Graph(show.GraphOptions{In: in, Role: role, Lang: lang, Max: max})
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&in, "in", "", "keep files whose path contains this substring")
	f.StringVar(&role, "role", "", "keep files with this role (impl|test|...)")
	f.StringVar(&lang, "lang", "", "keep files with this language")
	f.IntVar(&max, "max", 0, "cap the number of file nodes (0 = default 200)")
	return cmd
}
