#! /usr/bin/env bash
set -euo pipefail

# Read the docs to see how the project is structured and how to run it

# Style
# All var (and local var) references are fully wrapped in bracers

cd "$(dirname "$0")"

# A fresh clone needs its submodules before any go command will run: go.mod
# replaces github.com/tcpipuk/llama-go with third_party/llama-go, so that
# submodule (and llama.cpp nested inside it) has to be checked out even for a
# pure-Go build that never links it.
#
# This is the smart version of "git submodule update --init --recursive": it
# only touches git when something is actually missing, so it is cheap to call
# from the build, the test run and the dependencies command alike.
ensure_submodules() {
  # third_party/llama-go/go.mod proves the outer submodule is checked out;
  # llama.cpp/CMakeLists.txt proves the nested one is too.
  if [ -f "third_party/llama-go/go.mod" ] && [ -f "third_party/llama-go/llama.cpp/CMakeLists.txt" ]; then
    return 0
  fi

  echo "Initialising submodules (third_party/llama-go and llama.cpp)..."
  git submodule update --init --recursive
}

# Build the CLI for the current host into .build/cli/.
# The CLI is pure Go (built with CGO disabled), but it is only built for the
# host platform; the binary keeps the quesadilla-cli_<os>_<arch> naming.
build_cli() {
  ensure_submodules

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

# Build the llama.cpp static libraries that llama-go links against, inside the
# third_party/llama-go submodule. The embedded-llama backend is cgo, so any cgo
# build needs them. This is heavy (it builds llama.cpp with cmake) but only
# needs running once per platform. See docs/BACKENDS.md.
build_llama() {
  ensure_submodules

  echo "Building llama.cpp static libraries (third_party/llama-go)..."
  # On macOS llama-go's link flags reference the Metal and Accelerate (BLAS)
  # archives, which the Makefile only builds and copies with BUILD_TYPE=metal.
  # Its Metal object rule still expects an older, single-file layout, but the
  # archives already carry those symbols, so EXTRA_TARGETS is emptied to skip
  # that stale rule while keeping the archive copy.
  if [ "$(uname -s)" = "Darwin" ]; then
    make -C third_party/llama-go BUILD_TYPE=metal EXTRA_TARGETS= libbinding.a
  else
    make -C third_party/llama-go libbinding.a
  fi
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

  # The GUI is a cgo build, so it embeds the cgo embedded-llama backend and
  # needs llama.cpp's static libraries.
  (build_llama)

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

# Fyne on Windows builds through cgo with a MinGW-w64 gcc. Git for Windows
# ships one on the PATH of its bash; MSYS2's mingw64 is used as a fallback and,
# under GitHub Actions, added to PATH for the steps that follow.
dependencies_windows() {
  if command -v gcc >/dev/null 2>&1; then
    echo "Found $(gcc --version | head -n 1)"
    return 0
  fi

  local mingw_bin="C:/msys64/mingw64/bin"
  if [ -d "${mingw_bin}" ]; then
    export PATH="${mingw_bin}:${PATH}"
    if [ -n "${GITHUB_PATH:-}" ]; then
      echo "${mingw_bin}" >> "${GITHUB_PATH}"
    fi
    echo "Using MinGW-w64 from ${mingw_bin}"
    return 0
  fi

  echo "Error: no C compiler found. Install MinGW-w64 (e.g. via MSYS2) to build the GUI." >&2
  return 1
}

# Install the native build dependencies for the current platform by
# dispatching to the matching dependencies:<os> command. This is also where the
# fresh-clone submodule check lives, so `./run.sh dependencies` leaves the tree
# ready for a build.
dependencies() {
  ensure_submodules

  local goos
  goos="$(current_goos)"
  case "${goos}" in
    darwin) (dependencies_macos) ;;
    linux) (dependencies_linux) ;;
    windows) (dependencies_windows) ;;
    *)
      echo "Error: unsupported host for dependencies: ${goos}" >&2
      return 1
      ;;
  esac
}

# Build every target
build() {
  # Checked here too so the fresh-clone submodule requirement is handled up
  # front; ensure_submodules is a no-op once they are present.
  ensure_submodules

  build_cli
  build_macos
}

test() {
  # A cgo test binary links the embedded-llama backend, so the native
  # libraries have to exist before `go test ./...`.
  build_llama

  echo "Running tests..."
  go test ./...
}

clean() {
  echo "Cleaning generated files..."
  rm -rf .build
  find . -name ".DS_Store" -type f -exec rm -f {} + 2>/dev/null || true
  echo "Clean complete."
}

# Download act binaries for all platforms
# Downloads from https://github.com/nektos/act/releases/tag/v0.2.89
download_act() {
  local act_dir=".act"
  local version="v0.2.89"
  local base_url="https://github.com/nektos/act/releases/download/${version}"

  echo "Downloading act ${version} binaries to ${act_dir}/..."
  mkdir -p "${act_dir}"
  cd "${act_dir}"

  # Download all platform binaries
  echo "Downloading Linux amd64..."
  curl -L -s -o act_linux_amd64.tar.gz "${base_url}/act_Linux_x86_64.tar.gz"
  echo "Downloading Linux arm64..."
  curl -L -s -o act_linux_arm64.tar.gz "${base_url}/act_Linux_arm64.tar.gz"
  echo "Downloading macOS amd64..."
  curl -L -s -o act_darwin_amd64.tar.gz "${base_url}/act_Darwin_x86_64.tar.gz"
  echo "Downloading macOS arm64..."
  curl -L -s -o act_darwin_arm64.tar.gz "${base_url}/act_Darwin_arm64.tar.gz"
  echo "Downloading Windows amd64..."
  curl -L -s -o act_windows_amd64.zip "${base_url}/act_Windows_x86_64.zip"

  # Extract and rename
  echo "Extracting..."
  tar -xzf act_linux_amd64.tar.gz && mv act act_linux_amd64
  tar -xzf act_linux_arm64.tar.gz && mv act act_linux_arm64
  tar -xzf act_darwin_amd64.tar.gz && mv act act_darwin_amd64
  tar -xzf act_darwin_arm64.tar.gz && mv act act_darwin_arm64
  unzip -o act_windows_amd64.zip >/dev/null && mv act.exe act_windows_amd64.exe

  # Set execute permissions
  chmod +x act_linux_amd64 act_linux_arm64 act_darwin_amd64 act_darwin_arm64 act_windows_amd64.exe

  # Clean up archives and extra files
  rm -f act_linux_amd64.tar.gz act_linux_arm64.tar.gz act_darwin_amd64.tar.gz act_darwin_arm64.tar.gz act_windows_amd64.zip
  rm -f LICENSE README.md

  cd ..
  echo "Downloaded act binaries to ${act_dir}/"
  ls -la "${act_dir}/"
}

# Run act (GitHub Actions local runner)
# Usage: ./run.sh act [args...]
act() {
  local act_dir=".act"
  local os_arch
  local binary_name

  case "$(uname -s)" in
    Linux)
      case "$(uname -m)" in
        x86_64) os_arch="linux_amd64" ;;
        aarch64|arm64) os_arch="linux_arm64" ;;
        *) os_arch="linux_$(uname -m)" ;;
      esac
      ;;
    Darwin)
      case "$(uname -m)" in
        x86_64) os_arch="darwin_amd64" ;;
        arm64) os_arch="darwin_arm64" ;;
        *) os_arch="darwin_$(uname -m)" ;;
      esac
      ;;
    CYGWIN*|MINGW*|MSYS*)
      case "$(uname -m)" in
        x86_64) os_arch="windows_amd64" ;;
        *) os_arch="windows_$(uname -m)" ;;
      esac
      ;;
    *)
      echo "Error: Unsupported OS: $(uname -s)" >&2
      echo "Download act from https://github.com/nektos/act/releases" >&2
      return 1
      ;;
  esac

  if [[ "${os_arch}" == windows_* ]]; then
    binary_name="act_${os_arch}.exe"
  else
    binary_name="act_${os_arch}"
  fi

  local binary_path="${act_dir}/${binary_name}"

  if [[ ! -f "${binary_path}" ]]; then
    echo "act binary not found at ${binary_path}, downloading..." >&2
    (download_act)
    # Re-check after download
    if [[ ! -f "${binary_path}" ]]; then
      echo "Error: Failed to download act binary" >&2
      return 1
    fi
  fi

  if [[ ! -x "${binary_path}" ]]; then
    chmod +x "${binary_path}"
  fi

  mkdir -p ./.act/outputs
  exec "${binary_path}" --artifact-server-path ./.act/outputs "$@"
}

help() {
  cat <<EOF
Usage: ./run.sh <command>

Commands:
  help                 Show this help message
  download_act         Download act binaries for all platforms to .act/
  act                  Run GitHub Actions locally via act (passes args to act)
  dependencies         Install the native build dependencies for the host and fetch the llama-go/llama.cpp submodules
  dependencies:macos   Install the native build dependencies for macOS (Xcode command line tools)
  dependencies:linux   Install the native build dependencies for Linux (gcc + OpenGL/X11/xkbcommon/Wayland headers)
  dependencies:windows Install the native build dependencies for Windows (MinGW-w64 gcc)
  build                Build every target (host CLI, macOS .app)
  build:cli            Build the CLI for the current host into .build/cli/
  build:macos          Build the macOS GUI into .build/macos/Quesadilla.app (requires macOS)
  build:llama          Build llama.cpp's static libraries for the embedded-llama backend
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
  build:llama)
    (build_llama)
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
  dependencies:windows)
    (dependencies_windows)
    ;;
  test)
    (test)
    ;;
  clean)
    (clean)
    ;;
  download_act)
    (download_act)
    ;;
  act)
    shift
    (act "$@")
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
