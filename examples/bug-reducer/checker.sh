#!/bin/sh
set -eu

printf 'Checking candidate (%s bytes)\n' "$(wc -c < "$1")"
# Slow the demo down enough to see the dashboard update.
sleep "${CYCLO_DEMO_DELAY:-0.15}"

if ! grep -Fxq 'mode = legacy' "$1"; then
  printf 'Rejected: legacy mode is absent.\n'
  exit 1
fi

if ! grep -Fxq 'trigger = duplicate' "$1"; then
  printf 'Rejected: duplicate trigger is absent.\n'
  exit 1
fi

printf 'Accepted: both trigger lines remain; the demo bug reproduces.\n'
