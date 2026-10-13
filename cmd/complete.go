package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// isEnvFileName matches .env, .env.local, .env.example, production.env, ...
func isEnvFileName(name string) bool {
	return name == ".env" || strings.HasPrefix(name, ".env.") || strings.HasSuffix(name, ".env")
}

// completeEnvFiles suggests .env files, which shells otherwise hide because
// they are dotfiles. It falls back to normal file completion when the
// directory has none.
func completeEnvFiles(maxArgs int) cobra.CompletionFunc {
	return func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if maxArgs >= 0 && len(args) >= maxArgs {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return envFileCandidates(toComplete), cobra.ShellCompDirectiveDefault
	}
}

func completeEnvFileFlag(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return envFileCandidates(toComplete), cobra.ShellCompDirectiveDefault
}

func envFileCandidates(toComplete string) []string {
	dir, prefix := filepath.Split(toComplete)
	readDir := dir
	if readDir == "" {
		readDir = "."
	}
	entries, err := os.ReadDir(readDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, prefix) || !isEnvFileName(name) {
			continue
		}
		out = append(out, dir+name)
	}
	return out
}
