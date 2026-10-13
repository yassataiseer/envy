// Package cmd implements the envy command-line interface.
package cmd

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// version is overridden at build time with
// -ldflags "-X github.com/yassataiseer/envy/cmd.version=v0.1.0".
var version = "dev"

// Exit codes shared by all commands.
const (
	ExitOK    = 0
	ExitDrift = 1 // differences or audit violations found
	ExitError = 2 // parse, I/O, or usage error
)

// exitError carries a specific process exit code alongside an error.
// A nil Err exits silently with Code.
type exitError struct {
	Code int
	Err  error
}

func (e *exitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit status %d", e.Code)
	}
	return e.Err.Error()
}

func (e *exitError) Unwrap() error { return e.Err }

var rootCmd = &cobra.Command{
	Use:           "envy",
	Short:         "Diff, validate, and inject .env files",
	Long:          "Envy is a terminal-native tool for diffing, auditing, and safely injecting environment variables from .env files.",
	Version:       resolveVersion(),
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command and exits the process with the
// appropriate exit code.
func Execute() {
	err := rootCmd.Execute()
	if err == nil {
		return
	}
	code := ExitError
	var ee *exitError
	if errors.As(err, &ee) {
		code = ee.Code
		err = ee.Err
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "envy:", err)
	}
	os.Exit(code)
}

func resolveVersion() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}
