#!/bin/sh
# Takumi installer
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/T-Fizz/takumi/main/install.sh | sh
#
# Options (via env vars):
#   TAKUMI_INSTALL_DIR  — override install directory (default: /usr/local/bin or ~/.local/bin)
#   TAKUMI_VERSION      — override version (default: latest)

set -eu

REPO="T-Fizz/takumi"
BINARY="takumi"

# --- Detect OS and arch ---------------------------------------------------

detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
        *) echo "unsupported" ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *) echo "unsupported" ;;
    esac
}

OS=$(detect_os)
ARCH=$(detect_arch)

if [ "$OS" = "unsupported" ] || [ "$ARCH" = "unsupported" ]; then
    echo "Error: unsupported platform $(uname -s)/$(uname -m)"
    exit 1
fi

# --- Determine version ----------------------------------------------------

if [ -n "${TAKUMI_VERSION:-}" ]; then
    VERSION="$TAKUMI_VERSION"
else
    VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed 's/.*"tag_name": *"//;s/".*//')
    if [ -z "$VERSION" ]; then
        echo "Error: could not determine latest version"
        exit 1
    fi
fi

VERSION_NUM="${VERSION#v}"

# --- Determine install directory -------------------------------------------

default_install_dir() {
    if [ -w "/usr/local/bin" ]; then
        echo "/usr/local/bin"
    elif [ -d "$HOME/.local/bin" ] || mkdir -p "$HOME/.local/bin" 2>/dev/null; then
        echo "$HOME/.local/bin"
    else
        echo "/usr/local/bin"
    fi
}

INSTALL_DIR="${TAKUMI_INSTALL_DIR:-$(default_install_dir)}"

# --- Download and install --------------------------------------------------

EXT="tar.gz"
if [ "$OS" = "windows" ]; then
    EXT="zip"
fi

ARCHIVE="${BINARY}_${VERSION_NUM}_${OS}_${ARCH}.${EXT}"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"

echo "Installing takumi ${VERSION} (${OS}/${ARCH})"
echo "  from: ${URL}"
echo "  to:   ${INSTALL_DIR}"
echo ""

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading..."
if ! curl -fsSL "$URL" -o "$TMPDIR/$ARCHIVE"; then
    echo "Error: download failed. Check that ${VERSION} exists for ${OS}/${ARCH}."
    exit 1
fi

echo "Extracting..."
if [ "$EXT" = "zip" ]; then
    unzip -q "$TMPDIR/$ARCHIVE" -d "$TMPDIR"
else
    tar xzf "$TMPDIR/$ARCHIVE" -C "$TMPDIR"
fi

# --- Install binary and symlink --------------------------------------------

NEEDS_SUDO=""
if [ ! -w "$INSTALL_DIR" ]; then
    NEEDS_SUDO="sudo"
    echo "Note: ${INSTALL_DIR} requires elevated permissions."
fi

$NEEDS_SUDO mkdir -p "$INSTALL_DIR"
$NEEDS_SUDO cp "$TMPDIR/takumi" "$INSTALL_DIR/takumi"
$NEEDS_SUDO chmod +x "$INSTALL_DIR/takumi"

# Create t symlink
$NEEDS_SUDO ln -sf "$INSTALL_DIR/takumi" "$INSTALL_DIR/t"

echo ""
echo "Installed:"
echo "  ${INSTALL_DIR}/takumi"
echo "  ${INSTALL_DIR}/t -> takumi"

# --- Verify ----------------------------------------------------------------

if command -v takumi >/dev/null 2>&1; then
    echo ""
    echo "$(takumi --help 2>&1 | head -1)"
    echo ""
    echo "Done. Run 'takumi init' to get started."
elif echo "$PATH" | grep -q "$INSTALL_DIR"; then
    echo ""
    echo "Done. Run 'takumi init' to get started."
else
    echo ""
    echo "Done. Add ${INSTALL_DIR} to your PATH:"
    echo ""
    echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
    echo ""
    echo "Then run 'takumi init' to get started."
fi
