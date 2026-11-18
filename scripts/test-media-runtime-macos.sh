#!/usr/bin/env bash
set -euo pipefail
runtime="${1:?usage: test-media-runtime-macos.sh media-runtime-directory}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
scratch="$(mktemp -d "${TMPDIR:-/tmp}/luckytv-probe-test.XXXXXX")"
trap 'rm -rf "$scratch"' EXIT
# Relocate the real runtime beside test binaries at both GUI and CLI positions.
bundle="$scratch/中文 bundle fixture/Contents"
mkdir -p "$bundle/MacOS/cli" "$bundle/Resources/agent-host"
cp -R "$runtime" "$bundle/Resources/agent-host/media-runtime"
(
  cd "$root/backend"
  go test -c ./internal/asset -o "$bundle/MacOS/LuckyTV"
  go test -c ./internal/mediatools -o "$bundle/MacOS/MediaProbe"
)
for executable in "$bundle/MacOS/LuckyTV" "$bundle/MacOS/cli/luckytv"; do
  if [[ "$executable" == */cli/luckytv ]]; then cp "$bundle/MacOS/LuckyTV" "$executable"; fi
  env -u CANVAS_FFPROBE_PATH PATH=/usr/bin:/bin:/usr/sbin:/sbin LUCKYTV_TEST_BUNDLED_MEDIA=1 \
    "$executable" -test.run '^TestBundledMediaMetadataWithoutSystemFFprobe$' -test.v
done
env -u CANVAS_FFPROBE_PATH PATH=/usr/bin:/bin:/usr/sbin:/sbin \
  "$bundle/MacOS/MediaProbe" -test.run '^TestProbeLocalDoesNotFetchPlaylistURLs$' -test.v
