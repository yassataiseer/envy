// Package runner builds a process environment from .env files and runs a
// command with it, without writing values to disk.
package runner

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yassataiseer/envy/pkg/envparse"
)

// Options controls how file values combine with the parent environment.
type Options struct {
	// Isolated starts from an empty environment instead of inheriting the
	// parent's. Bare `export KEY` lines still pass KEY through.
	Isolated bool
	// NoOverride keeps values already set in the parent environment
	// instead of replacing them with values from the files.
	NoOverride bool
}

// ErrNotFound reports that the command could not be found on PATH.
var ErrNotFound = errors.New("command not found")

// ExitStatusError carries the exit status of a command run by Exec on
// platforms that cannot replace the current process.
type ExitStatusError struct{ Code int }

func (e *ExitStatusError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

// BuildEnv merges files (later files override earlier ones) into parent,
// a list of "KEY=value" strings such as os.Environ(). Values are expanded
// against keys from earlier lines and files, then the parent environment.
//
// The result keeps the parent's order, followed by new keys in the order
// they first appear in the files.
func BuildEnv(files []*envparse.EnvFile, parent []string, opts Options) ([]string, error) {
	parentVals := make(map[string]string, len(parent))
	parentOrder := make([]string, 0, len(parent))
	for _, kv := range parent {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			continue
		}
		if _, dup := parentVals[k]; !dup {
			parentOrder = append(parentOrder, k)
		}
		parentVals[k] = v
	}

	fileVals := map[string]string{}
	var fileOrder []string
	set := func(k, v string) {
		if _, seen := fileVals[k]; !seen {
			fileOrder = append(fileOrder, k)
		}
		fileVals[k] = v
	}
	lookup := func(k string) (string, bool) {
		if v, ok := fileVals[k]; ok {
			return v, true
		}
		v, ok := parentVals[k]
		return v, ok
	}

	for _, f := range files {
		if err := envparse.Expand(f, lookup); err != nil {
			return nil, err
		}
		for i := range f.Entries {
			e := &f.Entries[i]
			if !e.IsAssignment() {
				continue
			}
			if e.IsBareExport {
				if v, ok := parentVals[e.Key]; ok && opts.Isolated {
					set(e.Key, v)
				}
				continue
			}
			if strings.IndexByte(e.Value, 0) >= 0 {
				return nil, fmt.Errorf("%s:%d: value of %s contains a NUL byte, which cannot be passed in an environment", f.Path, e.LineNumber, e.Key)
			}
			set(e.Key, e.Value)
		}
	}

	env := make([]string, 0, len(parentOrder)+len(fileOrder))
	if !opts.Isolated {
		for _, k := range parentOrder {
			v := parentVals[k]
			if fv, ok := fileVals[k]; ok && !opts.NoOverride {
				v = fv
			}
			env = append(env, k+"="+v)
		}
	}
	for _, k := range fileOrder {
		if _, inParent := parentVals[k]; inParent && !opts.Isolated {
			continue
		}
		env = append(env, k+"="+fileVals[k])
	}
	return env, nil
}

// lookupEnv returns the last value of key in env.
func lookupEnv(env []string, key string) string {
	val := ""
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			val = v
		}
	}
	return val
}
