#!/bin/sh
# Takumi uninstaller
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/T-Fizz/takumi/main/uninstall.sh | sh
#
# How it finds takumi:
#   1. TAKUMI_INSTALL_DIR env var (if set)
#   2. Resolves `command -v takumi` (finds it wherever it is in PATH)
#   3. Falls back to common locations: /usr/local/bin, ~/.local/bin
#
# This handles custom install locations and binaries that were moved after install.

set -eu

echo "Takumi uninstaller"
echo ""

# --- Locate the binary ----------------------------------------------------

find_takumi_dir() {
    # 1. Explicit override
    if [ -n "${TAKUMI_INSTALL_DIR:-}" ]; then
        if [ -f "$TAKUMI_INSTALL_DIR/takumi" ]; then
            echo "$TAKUMI_INSTALL_DIR"
            return
        fi
        echo ""
        return
    fi

    # 2. Find via PATH (works even if user moved it)
    if command -v takumi >/dev/null 2>&1; then
        FOUND=$(command -v takumi)
        # Resolve symlinks to get the real binary location
        if [ -L "$FOUND" ]; then
            # readlink -f isn't portable; use dirname of the link target
            FOUND=$(readlink "$FOUND" 2>/dev/null || echo "$FOUND")
        fi
        dirname "$FOUND"
        return
    fi

    # 3. Check common locations
    for dir in /usr/local/bin "$HOME/.local/bin"; do
        if [ -f "$dir/takumi" ]; then
            echo "$dir"
            return
        fi
    done

    echo ""
}

DIR=$(find_takumi_dir)

if [ -z "$DIR" ]; then
    echo "takumi not found. Nothing to uninstall."
    echo ""
    echo "If you installed to a custom location, set TAKUMI_INSTALL_DIR:"
    echo "  TAKUMI_INSTALL_DIR=/path/to/dir curl -fsSL ... | sh"
    exit 0
fi

echo "Found takumi in: ${DIR}"

# --- Check for Homebrew install -------------------------------------------

if command -v brew >/dev/null 2>&1; then
    BREW_PREFIX=$(brew --prefix 2>/dev/null || true)
    if brew list takumi >/dev/null 2>&1 && echo "$DIR" | grep -q "$BREW_PREFIX"; then
        echo ""
        echo "It looks like takumi was installed via Homebrew."
        echo "Please uninstall with:"
        echo ""
        echo "  brew uninstall takumi"
        echo ""
        exit 0
    fi
fi

# --- Remove files ---------------------------------------------------------

NEEDS_SUDO=""
if [ ! -w "$DIR" ]; then
    NEEDS_SUDO="sudo"
    echo "Note: ${DIR} requires elevated permissions."
fi

echo ""
echo "Removing:"

removed=0

if [ -f "$DIR/takumi" ]; then
    echo "  ${DIR}/takumi"
    $NEEDS_SUDO rm "$DIR/takumi"
    removed=$((removed + 1))
fi

# Remove 't' symlink only if it points to takumi (or to where takumi was)
if [ -L "$DIR/t" ]; then
    LINK_TARGET=$(readlink "$DIR/t" 2>/dev/null || true)
    case "$LINK_TARGET" in
        *takumi*)
            echo "  ${DIR}/t -> ${LINK_TARGET}"
            $NEEDS_SUDO rm "$DIR/t"
            removed=$((removed + 1))
            ;;
        *)
            echo "  skipping ${DIR}/t (points to ${LINK_TARGET}, not takumi)"
            ;;
    esac
elif [ -f "$DIR/t" ]; then
    # Standalone binary named 't' — only remove if it IS takumi
    if "$DIR/t" --help 2>&1 | grep -q "takumi" 2>/dev/null; then
        echo "  ${DIR}/t"
        $NEEDS_SUDO rm "$DIR/t"
        removed=$((removed + 1))
    fi
fi

if [ "$removed" -eq 0 ]; then
    echo "  nothing to remove"
fi

echo ""
echo "Done. takumi has been uninstalled."
