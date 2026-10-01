#!/usr/bin/env bash
# install.sh — build sa from source and install to ~/.local/bin
# Usage:  bash install.sh

set -euo pipefail

BINARY_NAME="sa"
INSTALL_DIR="${SA_INSTALL_DIR:-$HOME/.local/bin}"

echo "Building sa..."

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
"$GO_BIN" build -ldflags="-s -w" -o "$BINARY_NAME" ./cmd/sa/

# Install
mkdir -p "$INSTALL_DIR"
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
    echo "  sa add personal"
    echo "  sa add work --email work@example.com"
    echo "  sa list"
    echo "  sa personal"
  elif [ -n "$FOUND" ] && [ "$FOUND" != "$INSTALL_DIR/$BINARY_NAME" ]; then
    echo "Warning: 'sa' found at $FOUND (before $INSTALL_DIR/$BINARY_NAME in PATH)."
    echo "Move $INSTALL_DIR earlier in your PATH, or use the full path:"
    echo "  $INSTALL_DIR/sa add personal"
  else
    echo "Ready. Open a new shell or run:"
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
    echo "Then try: sa add personal"
  fi
fi
