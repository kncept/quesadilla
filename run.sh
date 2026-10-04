#! /usr/bin/env bash
set -euo pipefail

# Read the docs to see how the project is structured and how to run it

# Style
# All var (and local var) references are fully wrapped in bracers

cd "$(dirname "$0")"

# Supported OS/ARCH combos for the CLI (pure Go, built with CGO disabled)
CLI_TARGETS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
)

# Build the CLI for every supported OS/ARCH into .build/cli/
build_cli() {
  echo "Building CLI for supported OS/ARCH..."
  local out_dir=".build/cli"
  mkdir -p "${out_dir}"

  for target in "${CLI_TARGETS[@]}"; do
    local go_os="${target%%/*}"
    local go_arch="${target##*/}"
    local suffix=""
    if [ "${go_os}" = "windows" ]; then
      suffix=".exe"
    fi
    local out_name="quesadilla-cli_${go_os}_${go_arch}${suffix}"
    echo "Building ${out_name}..."
    CGO_ENABLED=0 GOOS="${go_os}" GOARCH="${go_arch}" \
      go build -o "${out_dir}/${out_name}" ./app/cli
  done

  echo "CLI builds complete in ${out_dir}/"
}

# Build the macOS GUI into .build/macos/. The GUI uses cgo (fyne/glfw +
# AppKit) so it can only be built for the host, on a macOS machine.
build_macos() {
  if [ "$(uname -s)" != "Darwin" ]; then
    echo "Skipping macOS GUI build (requires a macOS host)"
    return 0
  fi

  local host_arch
  case "$(uname -m)" in
    arm64) host_arch="arm64" ;;
    x86_64) host_arch="amd64" ;;
    *)
      echo "Error: unsupported host arch: $(uname -m)" >&2
      return 1
      ;;
  esac

  local out_dir=".build/macos"
  mkdir -p "${out_dir}"
  local out_name="quesadilla-macos_darwin_${host_arch}"
  echo "Building ${out_name} (host)..."
  CGO_ENABLED=1 go build -o "${out_dir}/${out_name}" ./app/macos

  echo "macOS GUI build complete in ${out_dir}/"
}

# Build every target
build() {
  build_cli
  build_macos
}

test() {
  echo "Running tests..."
  go test ./...
}

clean() {
  echo "Cleaning generated files..."
  rm -rf .build
  find . -name ".DS_Store" -type f -exec rm -f {} + 2>/dev/null || true
  echo "Clean complete."
}

help() {
  cat <<EOF
Usage: ./run.sh <command>

Commands:
  help         Show this help message
  build        Build every target (CLI for all supported OS/ARCH, macOS GUI for the host)
  build:cli    Build the CLI for every supported OS/ARCH into .build/cli/
  build:macos  Build the macOS GUI for the host into .build/macos/ (requires macOS)
  test         Run go tests (go test ./...)
  clean        Remove generated files (.build/) and .DS_Store files
EOF
}

# Run commands in a subshell () just incase a cd command fails
case "${1:-}" in
  build)
    (build)
    ;;
  build:cli)
    (build_cli)
    ;;
  build:macos)
    (build_macos)
    ;;
  test)
    (test)
    ;;
  clean)
    (clean)
    ;;
  help|-h|--help)
    (help)
    ;;
  *)
    echo "unknown command: ${1:-}" >&2
    echo
    (help)
    ;;
esac
