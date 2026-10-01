#!/usr/bin/env bash
# install.sh — build tyv from source and install to ~/.local/bin
# Usage:  bash install.sh

set -euo pipefail

BINARY_NAME="tyv"
INSTALL_DIR="${TYV_INSTALL_DIR:-$HOME/.local/bin}"

echo "Building tyv..."

# Find Go
GO_BIN=""
for candidate in \
  "$(command -v go 2>/dev/null)" \
  /opt/homebrew/bin/go \
  /usr/local/go/bin/go \
  /usr/local/bin/go; do
  if [ -x "$candidate" ]; then
    GO_BIN="$candidate"
    break
  fi
done

if [ -z "$GO_BIN" ]; then
  echo "Error: Go not found."
  echo ""
  echo "Install Go from https://go.dev/dl/ or via Homebrew:"
  echo "  brew install go"
  exit 1
fi

echo "  Go: $GO_BIN ($($GO_BIN version | awk '{print $3}'))"

# Build
"$GO_BIN" build -ldflags="-s -w" -o "$BINARY_NAME" ./cmd/tyv/

# Install
mkdir -p "$INSTALL_DIR"
# Remove first: overwriting a binary in place makes macOS kill the new one
# ("zsh: killed") because it caches the old file's code signature.
rm -f "$INSTALL_DIR/$BINARY_NAME"
mv "$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
chmod 755 "$INSTALL_DIR/$BINARY_NAME"

echo "  Installed: $INSTALL_DIR/$BINARY_NAME"
echo ""

# PATH check
if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
  echo "Note: $INSTALL_DIR is not in your PATH."
  echo ""
  echo "Add it by running one of:"
  echo "  echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.zshrc && source ~/.zshrc"
  echo "  echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.bashrc && source ~/.bashrc"
  echo ""
else
  # Verify the right binary will be found
  FOUND="$(command -v $BINARY_NAME 2>/dev/null || true)"
  if [ "$FOUND" = "$INSTALL_DIR/$BINARY_NAME" ]; then
    echo "Ready. Try:"
    echo "  tyv add personal"
    echo "  tyv add work --email work@example.com"
    echo "  tyv list"
    echo "  tyv personal"
  elif [ -n "$FOUND" ] && [ "$FOUND" != "$INSTALL_DIR/$BINARY_NAME" ]; then
    echo "Warning: 'tyv' found at $FOUND (before $INSTALL_DIR/$BINARY_NAME in PATH)."
    echo "Move $INSTALL_DIR earlier in your PATH, or use the full path:"
    echo "  $INSTALL_DIR/tyv add personal"
  else
    echo "Ready. Open a new shell or run:"
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
    echo "Then try: tyv add personal"
  fi
fi
