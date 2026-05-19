#!/bin/sh
set -eu

repo="danew/devsync"
version="${DEVSYNC_VERSION:-latest}"
bin_dir="${DEVSYNC_INSTALL_DIR:-$HOME/.local/bin}"

mkdir -p "$bin_dir"

echo "Install script placeholder for $repo ($version)."
echo "For now, build from source with:"
echo "  go install github.com/danew/devsync/cmd/devsync@latest"
echo "or from this checkout:"
echo "  make install"
echo "Target bin directory: $bin_dir"
