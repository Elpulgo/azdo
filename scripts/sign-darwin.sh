#!/usr/bin/env sh
# Replace Go's linker-signed signature on darwin binaries with a real ad-hoc
# signature. macOS Sequoia+ kills binaries that carry only the linker-signed
# flag, which is what `go build` produces when cross-compiling from Linux.
#
# No-op for non-darwin builds and when rcodesign is not installed (CI is
# expected to install it before invoking goreleaser; missing it locally is
# tolerated so `goreleaser build --snapshot` still works on a dev machine).
#
# Usage: sign-darwin.sh <binary-path>

set -e

binary="$1"

if [ -z "$binary" ]; then
    echo "sign-darwin: missing binary path argument" >&2
    exit 1
fi

case "$binary" in
    *darwin*) ;;
    *)
        # Not a darwin build — nothing to do.
        exit 0
        ;;
esac

if [ ! -f "$binary" ]; then
    echo "sign-darwin: file not found: $binary" >&2
    exit 1
fi

if ! command -v rcodesign >/dev/null 2>&1; then
    echo "sign-darwin: rcodesign not on PATH; leaving $binary linker-signed" >&2
    exit 0
fi

rcodesign sign "$binary"
echo "sign-darwin: re-signed $binary"
