#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DESKTOP_DIR="$ROOT_DIR/backend/cmd/desktop"
GO_DIR="${LUCKYTV_GO_DIR:-/tmp/luckytv-go.rpIfVN/go}"

HOST_OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
AGENT_TARGET="${LUCKYTV_WAILS_PLATFORM:-$HOST_OS/$(uname -m)}"
[[ "$AGENT_TARGET" != "darwin/x86_64" ]] || AGENT_TARGET="darwin/amd64"
[[ "$AGENT_TARGET" != "linux/x86_64" ]] || AGENT_TARGET="linux/amd64"
if [[ "$AGENT_TARGET" == linux/* && ( "$HOST_OS" != linux || "$AGENT_TARGET" != linux/amd64 || "$(uname -m)" != x86_64 ) ]]; then
  echo "Linux releases require a native Linux x64 host" >&2
  exit 1
fi
# A clean build host has no installed LuckyTV. Only an explicitly supplied
# encoder is a required input; the native probe packet is built below.
if [[ "$AGENT_TARGET" == darwin/* && -n "${LUCKYTV_FFMPEG_RUNTIME:-}" ]]; then
  [[ -x "$LUCKYTV_FFMPEG_RUNTIME" ]] || { echo "Explicit encoder is not executable: $LUCKYTV_FFMPEG_RUNTIME" >&2; exit 1; }
  ENCODER_LICENSE="${LUCKYTV_FFMPEG_LICENSE:-$(dirname "$LUCKYTV_FFMPEG_RUNTIME")/ffmpeg.LICENSE}"
  [[ -f "$ENCODER_LICENSE" ]] || { echo "Encoder license missing: $ENCODER_LICENSE" >&2; exit 1; }
fi
bun "$ROOT_DIR/scripts/package-agent-host.mjs" "$AGENT_TARGET" --verify-runtime

if [[ ! -f "$ROOT_DIR/VERSION" ]]; then
  echo "VERSION file is required" >&2
  exit 1
fi

VERSION_VALUE="$(tr -d '[:space:]' < "$ROOT_DIR/VERSION")"
if [[ ! "$VERSION_VALUE" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  echo "Invalid VERSION: $VERSION_VALUE" >&2
  exit 1
fi

if ! command -v go >/dev/null 2>&1 && [[ -x "$GO_DIR/bin/go" ]]; then
  export PATH="$GO_DIR/bin:$PATH"
fi

if ! command -v go >/dev/null 2>&1; then
  echo "Go is required (set LUCKYTV_GO_DIR when using a bundled toolchain)" >&2
  exit 1
fi

# Use the shared size gate default; an explicit LUCKYTV_WEB_BUDGET_MIB override
# applies consistently to local and CI builds.

if [[ "${LUCKYTV_SKIP_LOCAL_VERIFY:-}" != "1" ]]; then
  "$ROOT_DIR/scripts/verify-luckytv-local-release.sh"
fi

COMMIT_VALUE="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_TIME_VALUE="${CANVAS_BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
export CANVAS_BUILD_VERSION="$VERSION_VALUE"
export LUCKYTV_DIST_PLATFORM="${AGENT_TARGET/\//-}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"

LDFLAGS="-X luckytv/backend/internal/buildinfo.Version=$VERSION_VALUE -X luckytv/backend/internal/buildinfo.Commit=$COMMIT_VALUE -X luckytv/backend/internal/buildinfo.BuildTime=$BUILD_TIME_VALUE"
if [[ -n "${LUCKYTV_UPDATER_PUBLIC_KEY:-}" ]]; then
  UPDATER_LDFLAGS="$(
    cd "$ROOT_DIR/backend"
    go run ./cmd/update-release print-ldflags
  )"
  LDFLAGS="$LDFLAGS $UPDATER_LDFLAGS"
fi
if [[ -n "${LUCKYTV_EXTRA_LDFLAGS:-}" ]]; then
  LDFLAGS="$LDFLAGS $LUCKYTV_EXTRA_LDFLAGS"
fi

if [[ -n "${LUCKYTV_WAILS_PLATFORM:-}" ]]; then
  host_arch="$(uname -m)"
  case "$LUCKYTV_WAILS_PLATFORM" in
    darwin/amd64)
      if [[ "$host_arch" == "arm64" ]]; then
        export CGO_ENABLED=1
        export CGO_CFLAGS="${CGO_CFLAGS:+$CGO_CFLAGS }-arch x86_64"
        export CGO_LDFLAGS="${CGO_LDFLAGS:+$CGO_LDFLAGS }-arch x86_64"
      fi
      ;;
    darwin/arm64)
      if [[ "$host_arch" == "x86_64" ]]; then
        export CGO_ENABLED=1
        export CGO_CFLAGS="${CGO_CFLAGS:+$CGO_CFLAGS }-arch arm64"
        export CGO_LDFLAGS="${CGO_LDFLAGS:+$CGO_LDFLAGS }-arch arm64"
      fi
      ;;
  esac
fi

echo "Building LuckyTV $VERSION_VALUE ($COMMIT_VALUE)"

# The frontend embeds the source index, so the native packet must exist before
# Wails builds it. Stage the runtime outside the build output directory.
if [[ "$AGENT_TARGET" == darwin/* ]]; then
  MEDIA_RUNTIME_STAGE="$(mktemp -d "${TMPDIR:-/tmp}/luckytv-release-media.XXXXXX")"
  trap 'rm -rf "${MEDIA_RUNTIME_STAGE:-}"' EXIT
  if [[ -n "${LUCKYTV_MEDIA_RUNTIME:-}" ]]; then
    bun -e 'const fs=require("fs"),crypto=require("crypto");const [root,target]=process.argv.slice(1);const m=JSON.parse(fs.readFileSync(root+"/manifest.json","utf8"));if(m.target!==target||crypto.createHash("sha256").update(fs.readFileSync(root+"/ffprobe")).digest("hex")!==m.probeSHA256)throw Error("Media runtime target or checksum mismatch")' "$LUCKYTV_MEDIA_RUNTIME" "$AGENT_TARGET"
    cp -R "$LUCKYTV_MEDIA_RUNTIME" "$MEDIA_RUNTIME_STAGE/media-runtime"
  else
    bash "$ROOT_DIR/scripts/package-media-runtime-macos.sh" "$AGENT_TARGET" \
      "$MEDIA_RUNTIME_STAGE/media-runtime" "" \
      "$ROOT_DIR/dist/LuckyTV-${VERSION_VALUE}-ffprobe-${AGENT_TARGET/\//-}-sources.tar.gz"
  fi
fi

mkdir -p "$DESKTOP_DIR/build"
cp "$ROOT_DIR/assets/app-icon.png" "$DESKTOP_DIR/build/appicon.png"

# Wails v3 embeds cmd/desktop/frontend/dist with go:embed. frontend is disabled in
# wails.yaml because web/ sits outside this Go module, so the built bundle has to
# be staged here before the compile step runs.
WEB_DIST="$ROOT_DIR/web/dist"
if [[ ! -f "$WEB_DIST/index.html" ]]; then
  echo "web/dist/index.html is missing; run the web build before packaging a release" >&2
  exit 1
fi
rm -rf "$DESKTOP_DIR/frontend/dist"
mkdir -p "$DESKTOP_DIR/frontend/dist"
cp -R "$WEB_DIST/." "$DESKTOP_DIR/frontend/dist/"
# .gitkeep is tracked so the otherwise-empty embed directory survives a clone.
# Recreating it keeps a build from showing up as a deletion in git status.
: > "$DESKTOP_DIR/frontend/dist/.gitkeep"

# Wake takes linker flags from wails.yaml only: the CLI has no -ldflags flag and
# GOFLAGS is overridden by the -ldflags Wake emits itself.
MANIFEST="$ROOT_DIR/backend/wails.yaml"
bun "$ROOT_DIR/scripts/wails-manifest-patch.mjs" --manifest "$MANIFEST" \
  --version "$VERSION_VALUE" --ldflags "$LDFLAGS"
restore_manifest() {
  bun "$ROOT_DIR/scripts/wails-manifest-patch.mjs" --manifest "$MANIFEST" --restore || true
}
trap restore_manifest EXIT

WAKE_TARGETS="${LUCKYTV_WAILS_PLATFORM:-$AGENT_TARGET}"
WAKE_TAGS=()
# webkit2_41 selects the GTK4/WebKitGTK 4.1 ABI on Linux; other targets reject it.
if [[ "$AGENT_TARGET" == linux/amd64 ]]; then
  WAKE_TARGETS="linux/amd64"
  WAKE_TAGS=(--tags webkit2_41)
fi
(
  cd "$ROOT_DIR/backend"
  WAILS_EXP_USE_WAKE=1 go run github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0 build \
    --targets "$WAKE_TARGETS" ${WAKE_TAGS[@]+"${WAKE_TAGS[@]}"}
)

copy_bundled_plugin_dirs() {
  local dest_root="$1"
  local manifest_file="$ROOT_DIR/plugin-packages/bundled-plugin-dirs.txt"
  [[ -f "$manifest_file" ]] || { echo "Bundled plugin manifest missing: $manifest_file" >&2; exit 1; }
  rm -rf "$dest_root"
  mkdir -p "$dest_root"
  local copied=0
  local dir_name
  while IFS= read -r dir_name || [[ -n "$dir_name" ]]; do
    dir_name="${dir_name%%#*}"
    dir_name="$(printf '%s' "$dir_name" | tr -d '[:space:]')"
    [[ -n "$dir_name" ]] || continue
    local src_dir="$ROOT_DIR/plugin-packages/$dir_name"
    [[ -f "$src_dir/package.json" ]] || { echo "Bundled plugin source missing package.json: $src_dir" >&2; exit 1; }
    cp -R "$src_dir" "$dest_root/$dir_name"
    [[ -f "$dest_root/$dir_name/package.json" ]] || { echo "Bundled plugin copy missing package.json: $dest_root/$dir_name" >&2; exit 1; }
    copied=$((copied + 1))
  done < "$manifest_file"
  [[ "$copied" -gt 0 ]] || { echo "No bundled plugin directories listed in $manifest_file" >&2; exit 1; }
}

if [[ "$AGENT_TARGET" == linux/amd64 ]]; then
  LINUX_BUNDLE="$DESKTOP_DIR/build/bin/LuckyTV-linux"
  mkdir -p "$LINUX_BUNDLE/plugin-packages" "$LINUX_BUNDLE/cli"
  mv "$DESKTOP_DIR/build/bin/LuckyTV" "$LINUX_BUNDLE/LuckyTV"
  cp "$ROOT_DIR/plugin-packages/"*.luckytv-plugin "$LINUX_BUNDLE/plugin-packages/"
  cp "$ROOT_DIR/docs/linux-install.md" "$LINUX_BUNDLE/README.md"
  bun "$ROOT_DIR/scripts/package-agent-host.mjs" "$AGENT_TARGET" "$LINUX_BUNDLE/agent-host"
  copy_bundled_plugin_dirs "$LINUX_BUNDLE/agent-host/bundled-plugins"
  bun "$ROOT_DIR/scripts/distribution-notices.mjs" "${AGENT_TARGET/\//-}" "$LINUX_BUNDLE/agent-host/licenses" "$VERSION_VALUE"
  (
    cd "$ROOT_DIR/backend"
    go build -trimpath -ldflags "$LDFLAGS" -o "$LINUX_BUNDLE/cli/luckytv" ./cmd/luckytv
  )
  chmod 755 "$LINUX_BUNDLE/LuckyTV" "$LINUX_BUNDLE/cli/luckytv"
  echo "Release bundle: $LINUX_BUNDLE"
  exit 0
fi

# Official protocol packages are runtime dependencies. Finder launches use the
# bundle Resources directory and must never depend on the caller's cwd.
APP_BUNDLE="$DESKTOP_DIR/build/bin/LuckyTV.app"
PLUGIN_RESOURCE_DIR="$APP_BUNDLE/Contents/Resources/plugin-packages"
mkdir -p "$PLUGIN_RESOURCE_DIR"
cp "$ROOT_DIR/plugin-packages/"*.luckytv-plugin "$PLUGIN_RESOURCE_DIR/"

bun "$ROOT_DIR/scripts/package-agent-host.mjs" "$AGENT_TARGET" \
  "$APP_BUNDLE/Contents/Resources/agent-host"
copy_bundled_plugin_dirs "$APP_BUNDLE/Contents/Resources/agent-host/bundled-plugins"
cp -R "$MEDIA_RUNTIME_STAGE/media-runtime" "$APP_BUNDLE/Contents/Resources/agent-host/media-runtime"
bun "$ROOT_DIR/scripts/distribution-notices.mjs" "${AGENT_TARGET/\//-}" \
  "$APP_BUNDLE/Contents/Resources/agent-host/licenses" "$VERSION_VALUE"
# Reuse the explicitly supplied (or already installed) encoder, including its
# license. The native workflow adapter must not rely on a Finder-launched PATH.
ENCODER_SOURCE="${LUCKYTV_FFMPEG_RUNTIME:-/Applications/LuckyTV.app/Contents/Resources/agent-host/media-runtime/ffmpeg}"
if [[ -x "$ENCODER_SOURCE" ]]; then
  ENCODER_LICENSE="${LUCKYTV_FFMPEG_LICENSE:-$(dirname "$ENCODER_SOURCE")/ffmpeg.LICENSE}"
  [[ -f "$ENCODER_LICENSE" ]] || { echo "Encoder license missing: $ENCODER_LICENSE" >&2; exit 1; }
  ENCODER_ARCH="${AGENT_TARGET##*/}"
  [[ "$ENCODER_ARCH" != amd64 ]] || ENCODER_ARCH=x86_64
  file "$ENCODER_SOURCE" | grep -q "$ENCODER_ARCH" || { echo "Wrong encoder architecture" >&2; exit 1; }
  otool -L "$ENCODER_SOURCE" | tail -n +2 | while read -r dylib _; do
    case "$dylib" in /usr/lib/*|/System/Library/*) ;; *) echo "Non-system encoder dependency: $dylib" >&2; exit 1 ;; esac
  done
  ENCODER_DEST="$APP_BUNDLE/Contents/Resources/agent-host/media-runtime"
  cp "$ENCODER_SOURCE" "$ENCODER_DEST/ffmpeg"
  cp "$ENCODER_LICENSE" "$ENCODER_DEST/ffmpeg.LICENSE"
  if [[ -f "$(dirname "$ENCODER_SOURCE")/ffmpeg.README" ]]; then cp "$(dirname "$ENCODER_SOURCE")/ffmpeg.README" "$ENCODER_DEST/ffmpeg.README"; fi
  chmod 755 "$ENCODER_DEST/ffmpeg"
  codesign --force --sign - "$ENCODER_DEST/ffmpeg"
fi
if [[ "$AGENT_TARGET" == "darwin/$(uname -m)" || ( "$AGENT_TARGET" == darwin/amd64 && "$(uname -m)" == x86_64 ) ]]; then
  bash "$ROOT_DIR/scripts/test-media-runtime-macos.sh" "$APP_BUNDLE/Contents/Resources/agent-host/media-runtime"
fi

# The luckytv CLI ships inside the bundle. External agents (Codex, Claude Code,
# Cursor) connect through it with their own client credential, so the installed
# app must carry it -- PATH is not a prerequisite.
#
# It goes in Contents/MacOS/cli, not directly next to the app binary: macOS
# volumes are case-insensitive by default, so a file named luckytv beside LuckyTV
# is the same file and would overwrite the app binary.
CLI_BINARY="$APP_BUNDLE/Contents/MacOS/cli/luckytv"
mkdir -p "$(dirname "$CLI_BINARY")"
(
  cd "$ROOT_DIR/backend"
  if [[ -n "${LUCKYTV_WAILS_PLATFORM:-}" ]]; then
    GOOS="${LUCKYTV_WAILS_PLATFORM%%/*}" GOARCH="${LUCKYTV_WAILS_PLATFORM##*/}" \
      go build -trimpath -ldflags "$LDFLAGS" -o "$CLI_BINARY" ./cmd/luckytv
  else
    go build -trimpath -ldflags "$LDFLAGS" -o "$CLI_BINARY" ./cmd/luckytv
  fi
)
if [[ ! -x "$CLI_BINARY" ]]; then
  echo "luckytv CLI was not built into the bundle: $CLI_BINARY" >&2
  exit 1
fi
# Adding a Mach-O binary to Contents/MacOS invalidates the bundle signature.
# Sign the CLI the same ad-hoc way the bundle is signed; the --deep bundle
# signature below then covers it too.
if command -v codesign >/dev/null 2>&1; then
  codesign --force --sign - "$CLI_BINARY"
fi

# Keep the generated macOS bundle metadata aligned with the repository version.
APP_PLIST="$APP_BUNDLE/Contents/Info.plist"
if [[ -f "$APP_PLIST" ]] && command -v plutil >/dev/null 2>&1; then
  MACOS_VERSION="${VERSION_VALUE#v}"
  MACOS_VERSION="${MACOS_VERSION%%-*}"
  plutil -replace CFBundleShortVersionString -string "$MACOS_VERSION" "$APP_PLIST"
  plutil -replace CFBundleVersion -string "$MACOS_VERSION" "$APP_PLIST"
fi

# The plist edit and the bundled CLI both invalidate Wails' ad-hoc signature;
# sign the final bundle so the local install gate can verify it.
if command -v codesign >/dev/null 2>&1; then
  codesign --force --deep --sign - "$APP_BUNDLE"
fi

echo "Release bundle: $APP_BUNDLE"
