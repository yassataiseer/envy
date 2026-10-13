//go:build windows

package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
)

// Exec runs argv[0] with env as a child process, since Windows cannot
// replace the current process. Ctrl+C is left to the child, and its exit
// status is returned as an *ExitStatusError.
func Exec(argv, env []string) error {
	if len(argv) == 0 {
		return errors.New("no command given")
	}
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return fmt.Errorf("%s: %w", argv[0], ErrNotFound)
	}
	cmd := exec.Command(path, argv[1:]...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	signal.Ignore(os.Interrupt)
	defer signal.Reset(os.Interrupt)

	err = cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return &ExitStatusError{Code: ee.ExitCode()}
	}
	if err != nil {
		return err
	}
	return &ExitStatusError{Code: 0}
}
