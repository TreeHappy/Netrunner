#!/usr/bin/env bash
# Build and install ueberzugpp from source (no prebuilt releases exist).
# Used by the `mise run install-ueberzugpp` task; needs a Debian/Ubuntu
# system with apt for the build dependencies.
set -euo pipefail

PREFIX="${PREFIX:-/usr/local}"
SRC="$(mktemp -d)/ueberzugpp"
trap 'rm -rf "$(dirname "$SRC")"' EXIT

if command -v apt-get >/dev/null; then
  apt-get install -y \
    build-essential cmake ninja-build pkg-config g++ \
    libvips-dev libsixel-dev libchafa-dev libssl-dev libtbb-dev \
    libopencv-dev libxcb-image0-dev libxcb-util-dev libxcb-res0-dev
fi

git clone --depth 1 --recurse-submodules \
  https://github.com/jstkdng/ueberzugpp "$SRC"
cmake -S "$SRC" -B "$SRC/build" -DCMAKE_BUILD_TYPE=Release
cmake --build "$SRC/build" -j"$(nproc)"
cmake --install "$SRC/build" --prefix "$PREFIX"

command -v ueberzugpp && echo "ueberzugpp installed: $(command -v ueberzugpp)"
