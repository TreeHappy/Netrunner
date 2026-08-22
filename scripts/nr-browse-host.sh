#!/usr/bin/env bash
# Run on the HOST: exec nr-browse inside the podman container while
# forwarding the terminal-identifying environment so graphics-protocol
# detection works (Ghostty, kitty, WezTerm...).
# Usage: nr-browse-host.sh [nrbrowse args...]
set -euo pipefail

CTR="${NETRUNNER_CONTAINER:-netrunner}"
REPO_IN_CONTAINER="${NETRUNNER_REPO_PATH:-/root/boardy/netrunner}"

args=(-ti)
forward() { [ -n "${!1:-}" ] && args+=(-e "$1=${!1}"); }
forward TERM
forward TERM_PROGRAM
forward TERM_PROGRAM_VERSION
forward COLORTERM
forward KITTY_WINDOW_ID
forward GHOSTTY_RESOURCES_DIR
forward WEZTERM_EXECUTABLE
forward NETRUNNER_IMAGES
forward NETRUNNER_ART

exec podman exec "${args[@]}" "$CTR" /bin/bash "$REPO_IN_CONTAINER/scripts/nr-browse.sh" "$@"
