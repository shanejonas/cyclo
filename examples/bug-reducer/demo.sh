#!/bin/sh
set -eu

example_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
output_dir=$(mktemp -d "${TMPDIR:-/tmp}/cyclo-bug-reducer-demo.XXXXXX")
printf 'Demo output: %s/reduced.txt\n' "$output_dir"

cd "$example_dir/../.."
exec go run . bug-reducer --output "$output_dir/reduced.txt" "$@" \
  "$example_dir/input.txt" -- sh "$example_dir/checker.sh"
