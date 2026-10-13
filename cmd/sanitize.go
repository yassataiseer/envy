package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/yassataiseer/envy/pkg/envparse"
	"github.com/yassataiseer/envy/pkg/sanitize"
)

type sanitizeFlags struct {
	output   string
	mask     bool
	keepSafe bool
}

func newSanitizeCmd() *cobra.Command {
	var fl sanitizeFlags
	c := &cobra.Command{
		Use:   "sanitize <input> [-o <output>]",
		Short: "Generate a .env.example template from a real .env file",
		Long: `Copy a .env file with its keys, comments, and layout intact, but with values
removed, producing a template that is safe to commit. Commented-out
assignments (# OLD_KEY=...) are stripped too, as are overridden duplicates.

--keep-safe keeps values that do not look sensitive (PORT=3000,
NODE_ENV=development, URLs without passwords). --mask writes placeholders
like <secret>, <url>, or <number> instead of empty values.

Without -o the template is written to standard output.

Exit codes: 0 success, 2 I/O or parse error.`,
		Example: `  envy sanitize .env -o .env.example
  envy sanitize .env --keep-safe --mask`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeEnvFiles(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runSanitize(cmd, args[0], fl)
			if err != nil {
				return &exitError{Code: ExitError, Err: err}
			}
			return nil
		},
	}
	f := c.Flags()
	f.StringVarP(&fl.output, "output", "o", "", "write the template to `file` instead of standard output")
	f.BoolVar(&fl.mask, "mask", false, "replace values with type placeholders instead of empty strings")
	f.BoolVar(&fl.keepSafe, "keep-safe", false, "keep values that do not look sensitive")
	_ = c.RegisterFlagCompletionFunc("output", completeEnvFileFlag)
	return c
}

func runSanitize(cmd *cobra.Command, input string, fl sanitizeFlags) error {
	f, err := envparse.ParseFile(input)
	if err != nil {
		return err
	}
	if fl.output != "" && sameFile(input, fl.output) {
		return errors.New("output would overwrite the input file")
	}

	res := sanitize.Sanitize(f, sanitize.Options{Mask: fl.mask, KeepSafe: fl.keepSafe})
	if fl.output == "" {
		_, err := cmd.OutOrStdout().Write(f.Format())
		return err
	}
	if err := f.WriteFile(fl.output); err != nil {
		return err
	}
	msg := fmt.Sprintf("Wrote %s: %d %s stripped", fl.output, res.Stripped, plural(res.Stripped, "value"))
	if fl.keepSafe {
		msg += fmt.Sprintf(", %d kept", res.Kept)
	}
	if res.Comments > 0 {
		msg += fmt.Sprintf(", %d commented-out %s cleared", res.Comments, plural(res.Comments, "value"))
	}
	fmt.Fprintln(cmd.ErrOrStderr(), msg+".")
	return nil
}

func sameFile(a, b string) bool {
	ai, errA := os.Stat(a)
	bi, errB := os.Stat(b)
	if errA == nil && errB == nil {
		return os.SameFile(ai, bi)
	}
	absA, _ := filepath.Abs(a)
	absB, _ := filepath.Abs(b)
	return absA == absB
}
