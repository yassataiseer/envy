package cmd

import (
	"errors"
	"io/fs"
	"os"

	"github.com/spf13/cobra"

	"github.com/yassataiseer/envy/pkg/envparse"
	"github.com/yassataiseer/envy/pkg/runner"
)

// Shell conventions for commands that cannot be run.
const (
	exitNotExecutable = 126
	exitNotFound      = 127
)

type runFlags struct {
	envFiles   []string
	isolated   bool
	noOverride bool
}

func newRunCmd() *cobra.Command {
	var fl runFlags
	c := &cobra.Command{
		Use:   "run [--env FILE]... [--] <command> [args...]",
		Short: "Run a command with variables from .env files",
		Long: `Run a command with variables from one or more .env files injected into its
environment. Values go straight into the new process; nothing is written to
disk. On macOS and Linux envy replaces itself with the command, so signals and
the exit status are exactly the command's own.

Files are applied in order, later ones overriding earlier ones, and override
variables already set in your shell unless --no-override is given. ${VAR}
references resolve against earlier keys and then your shell's environment.

Exit status: the command's own; 126 if it is not executable, 127 if it is not
found, 2 if a .env file cannot be read or parsed.`,
		Example: `  envy run -- npm start
  envy run --env .env.production -- node dist/server.js
  envy run -e .env -e .env.local --isolated -- ./bin/worker`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRun(args, fl)
		},
	}
	f := c.Flags()
	f.SetInterspersed(false)
	f.StringArrayVarP(&fl.envFiles, "env", "e", []string{".env"}, "`file` to load; repeat to layer files")
	f.BoolVar(&fl.isolated, "isolated", false, "do not inherit the current environment (bare 'export KEY' lines still pass KEY through)")
	f.BoolVar(&fl.noOverride, "no-override", false, "keep variables already set in the current environment")
	c.MarkFlagsMutuallyExclusive("isolated", "no-override")
	return c
}

func runRun(argv []string, fl runFlags) error {
	files := make([]*envparse.EnvFile, 0, len(fl.envFiles))
	for _, p := range fl.envFiles {
		f, err := envparse.ParseFile(p)
		if err != nil {
			return &exitError{Code: ExitError, Err: err}
		}
		files = append(files, f)
	}
	env, err := runner.BuildEnv(files, os.Environ(), runner.Options{Isolated: fl.isolated, NoOverride: fl.noOverride})
	if err != nil {
		return &exitError{Code: ExitError, Err: err}
	}

	err = runner.Exec(argv, env)
	var status *runner.ExitStatusError
	switch {
	case errors.As(err, &status):
		return &exitError{Code: status.Code}
	case errors.Is(err, runner.ErrNotFound):
		return &exitError{Code: exitNotFound, Err: err}
	case errors.Is(err, fs.ErrPermission):
		return &exitError{Code: exitNotExecutable, Err: err}
	case err != nil:
		return &exitError{Code: exitNotExecutable, Err: err}
	}
	return nil
}
