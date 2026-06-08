package cli

import (
	"github.com/spf13/cobra"

	"github.com/orafaelfragoso/columbus/internal/index"
)

func newReindexCmd(env *Env) *cobra.Command {
	var full, changed, clean, status, noEmbed bool
	cmd := &cobra.Command{
		Use:   "reindex",
		Short: "Re-chunk and re-embed changes since the last index",
		Long: "Diff the working tree against the last indexed state and update only " +
			"what changed: re-chunk symbols and files and refresh their embeddings.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := indexMode(full, changed, clean, status)
			if err != nil {
				return err
			}
			res, warnings, err := runIndex(env, mode, noEmbed)
			if err != nil {
				return err
			}
			res.Warnings = append(res.Warnings, warnings...)
			return renderResult(env, res)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&full, "full", false, "reindex everything from scratch (memories preserved)")
	f.BoolVar(&changed, "changed", false, "fast path: only files dirty in the working tree")
	f.BoolVar(&clean, "clean", false, "drop all index data (preserves config and memories)")
	f.BoolVar(&status, "status", false, "report index state without writing")
	f.BoolVar(&noEmbed, "no-embed", false, "skip embeddings (metadata-only index)")
	return cmd
}

func indexMode(full, changed, clean, status bool) (index.Mode, error) {
	count := 0
	for _, b := range []bool{full, changed, clean, status} {
		if b {
			count++
		}
	}
	if count > 1 {
		return 0, errUsage("--full, --changed, --clean and --status are mutually exclusive")
	}
	switch {
	case full:
		return index.ModeFull, nil
	case changed:
		return index.ModeChanged, nil
	case clean:
		return index.ModeClean, nil
	case status:
		return index.ModeStatus, nil
	default:
		return index.ModeIncremental, nil
	}
}
