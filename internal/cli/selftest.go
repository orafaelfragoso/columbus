package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/rafaelfragoso/columbus/internal/contract"
	"github.com/rafaelfragoso/columbus/internal/render"
)

// selftestResult is a trivial payload used only to exercise the I/O contract
// end-to-end (success path) until real commands exist.
type selftestResult struct {
	Message string `json:"message"`
}

func (selftestResult) CommandName() string { return "_selftest" }

func (r selftestResult) RenderText(w io.Writer, _ render.Options) error {
	_, err := fmt.Fprintln(w, r.Message)
	return err
}

func (r selftestResult) RenderLLM(w io.Writer, _ render.Options) error {
	_, err := fmt.Fprintf(w, "# selftest\n\n%s\n", r.Message)
	return err
}

// newSelftestCmd is a hidden command that proves the success and error
// projections of the I/O contract. --fail CODE forces an error envelope with
// the given canonical code so exit-code mapping is testable.
func newSelftestCmd(env *Env) *cobra.Command {
	var failCode string
	cmd := &cobra.Command{
		Use:    "_selftest",
		Short:  "Internal: exercise the I/O contract",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if failCode != "" {
				return &contract.Error{
					Code:    contract.Code(failCode),
					Message: "selftest forced failure: " + failCode,
					Hint:    "this is a synthetic error",
				}
			}
			return renderResult(env, selftestResult{Message: "ok"})
		},
	}
	cmd.Flags().StringVar(&failCode, "fail", "", "force an error with the given canonical code")
	return cmd
}
