#!/usr/bin/env bash
# Builds PhoneDesk.app (Apple Silicon + Intel) and copies it to /Applications.
# Needs Go 1.23+ and the Xcode command line tools (xcode-select --install).
set -euo pipefail
cd "$(dirname "$0")/.."

if ! command -v go >/dev/null; then
  echo "Go is not installed. Get it with: brew install go" >&2
  exit 1
fi
export PATH="$PATH:$(go env GOPATH)/bin"
if ! command -v wails >/dev/null; then
  echo "Installing the Wails CLI…"
  go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2
fi

VERSION="${1:-0.1.0}"
go mod tidy
go test ./internal/...
wails build -clean -platform darwin/universal -ldflags "-X main.Version=$VERSION"

rm -rf /Applications/PhoneDesk.app
cp -R build/bin/PhoneDesk.app /Applications/
echo "Installed /Applications/PhoneDesk.app"
open /Applications/PhoneDesk.app
