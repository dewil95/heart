#!/usr/bin/env bash
# Builds the Go core (core/mobile) into app/libs/netmerge-core.aar with gomobile.
# Needs Go (version in core/go.mod), the Android SDK (ANDROID_HOME) and the NDK (ANDROID_NDK_HOME).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root/core"

# gomobile and gobind at the version pinned in core/go.mod.
go install golang.org/x/mobile/cmd/gomobile golang.org/x/mobile/cmd/gobind
export PATH="$(go env GOPATH)/bin:$PATH"

mkdir -p "$root/app/libs"
# arm64 and arm for phones, amd64 for the emulator. -androidapi matches the app's minSdk.
gomobile bind \
  -target=android/arm64,android/arm,android/amd64 \
  -androidapi 26 \
  -trimpath -ldflags="-s -w" \
  -o "$root/app/libs/netmerge-core.aar" \
  ./mobile
echo "built $root/app/libs/netmerge-core.aar"
