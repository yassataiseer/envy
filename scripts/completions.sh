#!/bin/sh
# Generates shell completion scripts for release archives.
set -e
rm -rf completions
mkdir completions
for shell in bash zsh fish; do
	go run . completion "$shell" >"completions/envy.$shell"
done
