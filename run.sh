#! /usr/bin/env bash
set -euo pipefail

# Read the docs to see how the project is structured and how to run it

# Style
# All var (and local var) references are fully wrapped in bracers

cd "$(dirname "$0")"

# Build the CLI for the current host into .build/cli/.
# The CLI is pure Go (built with CGO disabled), but it is only built for the
# host platform; the binary keeps the quesadilla-cli_<os>_<arch> naming.
build_cli() {
  local goos goarch
  goos="$(current_goos)"
  goarch="$(current_goarch)"

  if [ "${goos}" = "unknown" ] || [ "${goarch}" = "unknown" ]; then
    echo "Error: cannot detect host platform for build:cli (got ${goos}/${goarch})" >&2
    return 1
  fi

  local suffix=""
  if [ "${goos}" = "windows" ]; then
    suffix=".exe"
  fi

  local out_dir=".build/cli"
  local out_name="quesadilla-cli_${goos}_${goarch}${suffix}"
  echo "Building ${out_name}..."
  mkdir -p "${out_dir}"
  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
    go build -o "${out_dir}/${out_name}" ./cmd/cli

  echo "CLI build complete in ${out_dir}/"
}

# Build the macOS GUI into a .app bundle at .build/macos/Quesadilla.app.
# The GUI uses cgo (fyne/glfw + AppKit) so it can only be built for the host,
# on a macOS machine. The bundle icon (an avocado-green letter Q) is drawn by
# cmd/icongen and baked into an .icns with iconutil.
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

  local app_name="Quesadilla"
  local out_dir=".build/macos"
  local app_dir="${out_dir}/${app_name}.app"
  local iconset_dir="${out_dir}/${app_name}.iconset"

  echo "Building ${app_name}.app for ${host_arch}..."
  rm -rf "${app_dir}" "${iconset_dir}"
  mkdir -p "${app_dir}/Contents/MacOS" "${app_dir}/Contents/Resources"

  CGO_ENABLED=1 go build -o "${app_dir}/Contents/MacOS/${app_name}" ./cmd/macos

  echo "Generating app icon..."
  go run ./cmd/icongen "${iconset_dir}"
  iconutil -c icns "${iconset_dir}" -o "${app_dir}/Contents/Resources/${app_name}.icns"
  rm -rf "${iconset_dir}"

  cat > "${app_dir}/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleDevelopmentRegion</key>
	<string>en</string>
	<key>CFBundleDisplayName</key>
	<string>${app_name}</string>
	<key>CFBundleExecutable</key>
	<string>${app_name}</string>
	<key>CFBundleIconFile</key>
	<string>${app_name}</string>
	<key>CFBundleIdentifier</key>
	<string>com.kncept.quesadilla</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleName</key>
	<string>${app_name}</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>0.1.0</string>
	<key>CFBundleVersion</key>
	<string>1</string>
	<key>LSMinimumSystemVersion</key>
	<string>11.0</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>NSPrincipalClass</key>
	<string>NSApplication</string>
</dict>
</plist>
EOF

  echo "macOS GUI build complete: ${app_dir} (open it with 'open ${app_dir}')"
}

# Map the current host to a Go GOOS.
current_goos() {
  case "$(uname -s)" in
    Darwin) echo "darwin" ;;
    Linux) echo "linux" ;;
    *MINGW*|*MSYS*|*CYGWIN*) echo "windows" ;;
    *) echo "unknown" ;;
  esac
}

# Map the current host to a GOARCH.
current_goarch() {
  case "$(uname -m)" in
    arm64|aarch64) echo "arm64" ;;
    x86_64|amd64) echo "amd64" ;;
    *) echo "unknown" ;;
  esac
}

# Install the native libraries Fyne needs to build on macOS.
# Fyne links against AppKit through cgo, which needs the Xcode command line
# tools (they also provide iconutil, used for the .app icon).
dependencies_macos() {
  if xcode-select -p >/dev/null 2>&1; then
    echo "Xcode command line tools already installed: $(xcode-select -p)"
    return 0
  fi

  echo "Installing Xcode command line tools (required to build the Fyne GUI)..."
  xcode-select --install || true
  echo "Re-run './run.sh dependencies' once the installation finishes."
}

# Install the native libraries Fyne needs to build on Linux: a C compiler plus
# the OpenGL, X11, xkbcommon and Wayland development headers (GLFW needs the
# Wayland headers even for its X11 backend).
dependencies_linux() {
  if command -v apt-get >/dev/null 2>&1; then
    echo "Installing Fyne build dependencies with apt-get..."
    sudo apt-get update
    sudo apt-get install -y gcc libgl1-mesa-dev xorg-dev libxkbcommon-dev libwayland-dev
  elif command -v dnf >/dev/null 2>&1; then
    echo "Installing Fyne build dependencies with dnf..."
    sudo dnf install -y gcc libXcursor-devel libXrandr-devel mesa-libGL-devel \
      libXi-devel libXinerama-devel libXxf86vm-devel libxkbcommon-devel wayland-devel
  elif command -v pacman >/dev/null 2>&1; then
    echo "Installing Fyne build dependencies with pacman..."
    sudo pacman -S --noconfirm --needed gcc libx11 libxcursor libxrandr \
      libxinerama libxi libxxf86vm mesa libxkbcommon wayland
  else
    echo "Error: no supported package manager found (apt-get, dnf, or pacman)." >&2
    echo "Install a C compiler plus the OpenGL/X11/xkbcommon/Wayland development headers." >&2
    return 1
  fi
}

# Install the native build dependencies for the current platform by
# dispatching to the matching dependencies:<os> command.
dependencies() {
  local goos
  goos="$(current_goos)"
  case "${goos}" in
    darwin) (dependencies_macos) ;;
    linux) (dependencies_linux) ;;
    *)
      echo "Error: unsupported host for dependencies: ${goos}" >&2
      return 1
      ;;
  esac
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
  help                 Show this help message
  dependencies         Install the native build dependencies for the host (autoselects the OS)
  dependencies:macos   Install the native build dependencies for macOS (Xcode command line tools)
  dependencies:linux   Install the native build dependencies for Linux (gcc + OpenGL/X11/xkbcommon/Wayland headers)
  build                Build every target (host CLI, macOS .app)
  build:cli            Build the CLI for the current host into .build/cli/
  build:macos          Build the macOS GUI into .build/macos/Quesadilla.app (requires macOS)
  test                 Run go tests (go test ./...)
  clean                Remove generated files (.build/) and .DS_Store files
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
  dependencies)
    (dependencies)
    ;;
  dependencies:macos)
    (dependencies_macos)
    ;;
  dependencies:linux)
    (dependencies_linux)
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
