#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STAGED_APP="$ROOT_DIR/backend/cmd/desktop/build/bin/LuckyTV.app"
INSTALLED_APP="/Applications/LuckyTV.app"

cleanup_staged_app() {
  if [[ -d "$STAGED_APP" && ! -L "$STAGED_APP" ]]; then
    find "$STAGED_APP" -depth -delete
  fi
}
trap cleanup_staged_app EXIT

quit_installed_app() {
  osascript -e 'tell application id "com.wails.luckytv" to quit' 2>/dev/null || true
  for _ in 1 2 3 4 5; do
    pgrep -f '^/Applications/LuckyTV.app/Contents/MacOS/LuckyTV$' >/dev/null || return 0
    sleep 1
  done
  pgrep -f '^/Applications/LuckyTV.app/Contents/MacOS/LuckyTV$' >/dev/null || return 0
  echo "LuckyTV is still running. Save your work and quit before updating." >&2
  return 1
}

# Wails binding generation opens the workspace too, so release its lock first.
quit_installed_app
"$ROOT_DIR/scripts/build-luckytv-release.sh"

if [[ ! -x "$STAGED_APP/Contents/MacOS/LuckyTV" ]]; then
  echo "Built LuckyTV.app is incomplete: $STAGED_APP" >&2
  exit 1
fi
# External agents connect through the bundled CLI, so an install without it is
# incomplete even though the GUI would start.
if [[ ! -x "$STAGED_APP/Contents/MacOS/cli/luckytv" ]]; then
  echo "Built LuckyTV.app has no bundled luckytv CLI: $STAGED_APP/Contents/MacOS/cli/luckytv" >&2
  exit 1
fi
codesign --verify --deep --strict "$STAGED_APP"
if [[ -e "$INSTALLED_APP" && ( ! -d "$INSTALLED_APP" || -L "$INSTALLED_APP" ) ]]; then
  echo "Refusing to replace unexpected target: $INSTALLED_APP" >&2
  exit 1
fi

quit_installed_app

mkdir -p "$INSTALLED_APP"
rsync -a --delete "$STAGED_APP/" "$INSTALLED_APP/"
codesign --verify --deep --strict "$INSTALLED_APP"

echo "Updated the canonical local app: $INSTALLED_APP"
