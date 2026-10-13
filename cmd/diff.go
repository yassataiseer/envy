package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/yassataiseer/envy/pkg/diff"
	"github.com/yassataiseer/envy/pkg/envparse"
	"github.com/yassataiseer/envy/pkg/tui"
)

type diffFlags struct {
	interactive   bool
	noInteractive bool
	asJSON        bool
	keysOnly      bool
	showValues    bool
	reveal        bool
	highContrast  bool
}

func newDiffCmd() *cobra.Command {
	var fl diffFlags
	c := &cobra.Command{
		Use:   "diff <source> <target>",
		Short: "Compare two .env files",
		Long: `Compare two .env files key by key.

In an interactive terminal this opens a side-by-side view where you can copy
keys between files, delete keys, and save. Values are masked until you press m.

Otherwise it prints the differing keys. Values are omitted unless
--show-values is given.

Exit codes (non-interactive): 0 identical, 1 differences found, 2 error.`,
		Example: `  envy diff .env .env.example
  envy diff .env .env.example --keys-only --no-interactive
  envy diff .env.staging .env.production --json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiff(cmd, args[0], args[1], fl)
		},
	}
	f := c.Flags()
	f.BoolVarP(&fl.interactive, "interactive", "i", false, "always open the interactive view")
	f.BoolVar(&fl.noInteractive, "no-interactive", false, "never open the interactive view")
	f.BoolVar(&fl.asJSON, "json", false, "print the diff as JSON")
	f.BoolVarP(&fl.keysOnly, "keys-only", "k", false, "compare key presence only, ignoring values (useful against templates)")
	f.BoolVar(&fl.showValues, "show-values", false, "include values in text and JSON output")
	f.BoolVar(&fl.reveal, "reveal", false, "start the interactive view with values visible")
	f.BoolVar(&fl.highContrast, "high-contrast", false, "use a high-contrast color theme")
	c.MarkFlagsMutuallyExclusive("interactive", "no-interactive")
	c.MarkFlagsMutuallyExclusive("interactive", "json")
	return c
}

func runDiff(cmd *cobra.Command, sourcePath, targetPath string, fl diffFlags) error {
	left, err := envparse.ParseFile(sourcePath)
	if err != nil {
		return &exitError{Code: ExitError, Err: err}
	}
	right, err := envparse.ParseFile(targetPath)
	if err != nil {
		return &exitError{Code: ExitError, Err: err}
	}
	opts := diff.Options{IgnoreValues: fl.keysOnly}
	out := cmd.OutOrStdout()

	tty := isTerminal(cmd.InOrStdin()) && isTerminal(out)
	if fl.interactive && !tty {
		return &exitError{Code: ExitError, Err: errors.New("--interactive requires a terminal")}
	}
	if fl.interactive || (tty && !fl.noInteractive && !fl.asJSON) {
		_, err := tui.Run(tui.Config{
			Left:         left,
			Right:        right,
			Options:      opts,
			HighContrast: fl.highContrast,
			Reveal:       fl.reveal,
		}, cmd.InOrStdin(), out)
		if err != nil {
			return &exitError{Code: ExitError, Err: err}
		}
		return nil
	}

	set := diff.Compare(left, right, opts)
	if fl.asJSON {
		err = writeDiffJSON(out, set, fl.showValues)
	} else {
		err = writeDiffText(out, set, fl.showValues)
	}
	if err != nil {
		return &exitError{Code: ExitError, Err: err}
	}
	if set.HasDrift {
		return &exitError{Code: ExitDrift}
	}
	return nil
}

func isTerminal(v any) bool {
	f, ok := v.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

func writeDiffText(w io.Writer, set *diff.FileDiffSet, showValues bool) error {
	sum := set.Summary()
	if !set.HasDrift {
		_, err := fmt.Fprintf(w, "No differences between %s and %s (%d keys).\n", set.SourcePath, set.TargetPath, len(set.Results))
		return err
	}

	fmt.Fprintf(w, "--- %s\n+++ %s\n", set.SourcePath, set.TargetPath)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range set.Results {
		if r.Status == diff.StatusIdentical {
			continue
		}
		fmt.Fprintf(tw, "%s %s\t%s", r.Status.Marker(), r.Key, describe(r, set))
		if showValues {
			fmt.Fprintf(tw, "\t%s → %s", quoteEntry(r.LeftEntry), quoteEntry(r.RightEntry))
		}
		fmt.Fprintln(tw)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	var parts []string
	add := func(n int, label string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, label))
		}
	}
	add(sum.ValueDiff, "changed")
	add(sum.TypeMismatch, "type mismatch")
	add(sum.MissingRight, "missing in "+set.TargetPath)
	add(sum.MissingLeft, "missing in "+set.SourcePath)
	noun := "differences"
	if sum.Differences() == 1 {
		noun = "difference"
	}
	_, err := fmt.Fprintf(w, "\n%d %s: %s\n", sum.Differences(), noun, strings.Join(parts, ", "))
	return err
}

func describe(r diff.DiffResult, set *diff.FileDiffSet) string {
	switch r.Status {
	case diff.StatusValueDiff:
		return "value differs"
	case diff.StatusTypeMismatch:
		return fmt.Sprintf("type mismatch (%s vs %s)", r.LeftKind, r.RightKind)
	case diff.StatusMissingLeft:
		return "missing in " + set.SourcePath
	case diff.StatusMissingRight:
		return "missing in " + set.TargetPath
	}
	return "identical"
}

func quoteEntry(e *envparse.EnvEntry) string {
	if e == nil {
		return "(missing)"
	}
	return strconv.Quote(e.Value)
}

type jsonDiff struct {
	Source   string       `json:"source"`
	Target   string       `json:"target"`
	HasDrift bool         `json:"has_drift"`
	Summary  diff.Summary `json:"summary"`
	Results  []jsonResult `json:"results"`
}

type jsonSide struct {
	Line  int            `json:"line"`
	Kind  diff.ValueKind `json:"kind"`
	Value *string        `json:"value,omitempty"`
}

type jsonResult struct {
	Key    string          `json:"key"`
	Status diff.DiffStatus `json:"status"`
	Source *jsonSide       `json:"source,omitempty"`
	Target *jsonSide       `json:"target,omitempty"`
}

func writeDiffJSON(w io.Writer, set *diff.FileDiffSet, showValues bool) error {
	side := func(e *envparse.EnvEntry, kind diff.ValueKind) *jsonSide {
		if e == nil {
			return nil
		}
		s := &jsonSide{Line: e.LineNumber, Kind: kind}
		if showValues {
			v := e.Value
			s.Value = &v
		}
		return s
	}
	out := jsonDiff{
		Source:   set.SourcePath,
		Target:   set.TargetPath,
		HasDrift: set.HasDrift,
		Summary:  set.Summary(),
		Results:  make([]jsonResult, 0, len(set.Results)),
	}
	for _, r := range set.Results {
		out.Results = append(out.Results, jsonResult{
			Key:    r.Key,
			Status: r.Status,
			Source: side(r.LeftEntry, r.LeftKind),
			Target: side(r.RightEntry, r.RightKind),
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
