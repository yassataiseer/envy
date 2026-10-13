package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yassataiseer/envy/pkg/envparse"
)

func newParseCmd() *cobra.Command {
	var (
		asJSON bool
		expand bool
	)
	c := &cobra.Command{
		Use:    "parse <file|->",
		Short:  "Print the parsed structure of a .env file (debugging aid)",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := parseInput(cmd.InOrStdin(), args[0])
			if err != nil {
				return &exitError{Code: ExitError, Err: err}
			}
			if expand {
				if err := envparse.Expand(f, os.LookupEnv); err != nil {
					return &exitError{Code: ExitError, Err: err}
				}
			}
			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(f)
			}
			printEntries(out, f)
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "output the AST as JSON")
	c.Flags().BoolVar(&expand, "expand", false, "expand ${VAR} references using earlier keys and the process environment")
	return c
}

func parseInput(stdin io.Reader, path string) (*envparse.EnvFile, error) {
	if path == "-" {
		return envparse.Parse(stdin, "<stdin>")
	}
	return envparse.ParseFile(path)
}

func printEntries(w io.Writer, f *envparse.EnvFile) {
	for i := range f.Entries {
		e := &f.Entries[i]
		lines := strconv.Itoa(e.LineNumber)
		if e.EndLine != e.LineNumber {
			lines += "-" + strconv.Itoa(e.EndLine)
		}
		switch {
		case e.IsEmpty:
			fmt.Fprintf(w, "%-7s (blank)\n", lines)
		case e.IsComment:
			fmt.Fprintf(w, "%-7s %s\n", lines, e.RawComment)
		default:
			var tags []string
			if e.QuoteType != envparse.QuoteNone {
				tags = append(tags, e.QuoteType.String())
			}
			if e.IsBareExport {
				tags = append(tags, "bare export")
			} else if e.IsExported {
				tags = append(tags, "exported")
			}
			if e.RawComment != "" {
				tags = append(tags, "comment="+strconv.Quote(e.RawComment))
			}
			suffix := ""
			if len(tags) > 0 {
				suffix = "  [" + strings.Join(tags, ", ") + "]"
			}
			fmt.Fprintf(w, "%-7s %s=%s%s\n", lines, e.Key, strconv.Quote(e.Value), suffix)
		}
	}
}
