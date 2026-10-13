//go:build !windows

package runner

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Exec replaces the current process with argv[0], run with env. On
// success it does not return: signals, the terminal, and the exit status
// all belong to the command, and no envy process stays resident holding
// the values. The command is resolved against the PATH in env, so a PATH
// set in a .env file takes effect.
func Exec(argv, env []string) error {
	if len(argv) == 0 {
		return errors.New("no command given")
	}
	path, err := lookPath(argv[0], lookupEnv(env, "PATH"))
	if err != nil {
		return err
	}
	if err := syscall.Exec(path, argv, env); err != nil {
		return fmt.Errorf("exec %s: %w", argv[0], err)
	}
	return nil
}

func lookPath(file, pathEnv string) (string, error) {
	if strings.Contains(file, "/") {
		if err := checkExecutable(file); err != nil {
			return "", err
		}
		return file, nil
	}
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			dir = "."
		}
		p := filepath.Join(dir, file)
		if checkExecutable(p) == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s: %w", file, ErrNotFound)
}

func checkExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s: %w", path, ErrNotFound)
		}
		return err
	}
	if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%s: %w", path, fs.ErrPermission)
	}
	return nil
}
