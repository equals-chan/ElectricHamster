#!/usr/bin/env bash
#
# Build the Windows ElectricHamster executable and (optionally) an NSIS
# installer. Designed to run from WSL/Linux with the Windows toolchain
# available (Go cross-compile + Node for the frontend + makensis for the
# installer).
#
# Usage:
#   scripts/package-windows.sh            # exe + portable zip + installer
#   scripts/package-windows.sh exe        # just the .exe + portable zip
#   scripts/package-windows.sh installer  # exe + NSIS installer
#
# Environment knobs:
#   APP_NAME=ElectricHamster   BIN_DIR=bin   ARCH=amd64
#   INSTALL_SCOPE=user|machine (NSIS default: user)
#   SKIP_FRONTEND=1            skip `npm run build`
#
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

APP_NAME="${APP_NAME:-ElectricHamster}"
BIN_DIR="${BIN_DIR:-bin}"
ARCH="${ARCH:-amd64}"
INSTALL_SCOPE="${INSTALL_SCOPE:-user}"
TARGET="${1:-all}"

# Make sure the local toolchains installed earlier are on PATH (best effort).
export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$HOME/go/bin:$PATH"

log()  { printf '\033[36m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[33mwarning:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "$1 not found in PATH ($2)"; }
need go   "install Go"
need node "install Node.js"

# makensis may live on the Windows side (e.g. scoop shim) when running from WSL.
if ! command -v makensis >/dev/null 2>&1; then
  for cand in \
    "/mnt/c/Users/$USER/scoop/shims/makensis.exe" \
    "/mnt/c/Users/admin/scoop/shims/makensis.exe" \
    "/mnt/c/Program Files (x86)/NSIS/makensis.exe" \
    "/mnt/c/Program Files/NSIS/makensis.exe"; do
    if [[ -x "$cand" ]]; then
      makensis() { "$cand" "$@"; }
      export -f makensis 2>/dev/null || true
      MAKENSIS_BIN="$cand"
      break
    fi
  done
fi
MAKENSIS_BIN="${MAKENSIS_BIN:-makensis}"

GOARCH_BUILD="$ARCH"
EXE="$BIN_DIR/$APP_NAME.exe"
mkdir -p "$BIN_DIR"

# --- 1. embedded 7-Zip runtime ---------------------------------------------
RUNTIME_DIR="internal/archive/runtime/windows-amd64"
if [[ ! -f "$RUNTIME_DIR/7za.exe" && ! -f "$RUNTIME_DIR/7z.exe" ]]; then
  warn "no 7-Zip binary in $RUNTIME_DIR; building without embedded engine"
  warn "  the app will fall back to a system 7-Zip / \$EH_SEVENZIP at runtime"
  EMBED_TAGS="production"
else
  EMBED_TAGS="production,embed_runtime"
fi

# --- 2. frontend ------------------------------------------------------------
if [[ "${SKIP_FRONTEND:-0}" != "1" ]]; then
  log "building frontend"
  ( cd frontend && npm install --no-audit --no-fund >/dev/null && npm run build )
else
  log "skipping frontend build (SKIP_FRONTEND=1)"
fi
[[ -f frontend/dist/index.html ]] || die "frontend/dist is empty; run the frontend build first"

# --- 3. Windows executable --------------------------------------------------
log "compiling $EXE (tags: $EMBED_TAGS)"
rm -f "$EXE"

SYSO="wails_windows_${ARCH}.syso"
if command -v wails3 >/dev/null 2>&1 && [[ -f build/windows/icon.ico ]]; then
  # Regenerate icon.ico from build/appicon.png so icon changes take effect.
  if [[ -f build/appicon.png ]]; then
    log "generating Windows icon from build/appicon.png"
    ( cd build && wails3 generate icons -input appicon.png \
        -windowsfilename windows/icon.ico >/dev/null 2>&1 ) \
      || warn "icon generation failed (possibly missing darwin assets); using existing icon.ico"
  fi
  log "generating Windows resources ($SYSO)"
  ( cd build && wails3 generate syso -arch "$ARCH" \
      -icon windows/icon.ico \
      -manifest windows/wails.exe.manifest \
      -info windows/info.json \
      -out "../$SYSO" ) || warn "syso generation failed; building without icon/manifest"
  [[ -f "$SYSO" ]] || warn "expected $SYSO was not produced; exe will have the default icon"
else
  warn "wails3 not found; skipping .syso (no custom icon/manifest)"
fi

GOOS=windows GOARCH="$GOARCH_BUILD" CGO_ENABLED=0 go build \
  -tags "$EMBED_TAGS" \
  -trimpath -buildvcs=false \
  -ldflags="-w -s -H windowsgui" \
  -o "$EXE" .

rm -f "$SYSO"
SIZE=$(du -h "$EXE" | cut -f1)
log "built $EXE ($SIZE)"

# --- 4. portable zip --------------------------------------------------------
ZIP="$BIN_DIR/${APP_NAME}-${ARCH}-portable.zip"
if command -v zip >/dev/null 2>&1; then
  log "creating portable archive $ZIP"
  ( cd "$BIN_DIR" && rm -f "$(basename "$ZIP")" && zip -q "$(basename "$ZIP")" "$(basename "$EXE")" )
else
  warn "zip not found; skipping portable archive"
fi

# --- 5. NSIS installer ------------------------------------------------------
if [[ "$TARGET" == "all" || "$TARGET" == "installer" ]]; then
  if command -v "$MAKENSIS_BIN" >/dev/null 2>&1 || [[ -x "$MAKENSIS_BIN" ]]; then
    log "building NSIS installer (scope: $INSTALL_SCOPE)"
    # A WSL path like /mnt/c/... must be converted for the Windows makensis.
    if command -v wslpath >/dev/null 2>&1 && [[ "$MAKENSIS_BIN" == /mnt/c/* ]]; then
      NSIS_DEFINES=(-DARG_WAILS_AMD64_BINARY="$(wslpath -w "$ROOT_DIR/$EXE")")
      NSIS_DIR="$(wslpath -w "$ROOT_DIR/build/windows/nsis")"
    else
      NSIS_DEFINES=(-DARG_WAILS_AMD64_BINARY="$ROOT_DIR/$EXE")
      NSIS_DIR="$ROOT_DIR/build/windows/nsis"
    fi

    if command -v wails3 >/dev/null 2>&1; then
      wails3 generate webview2bootstrapper -dir build/windows/nsis >/dev/null 2>&1 || \
        warn "could not fetch WebView2 bootstrapper; installer will not bundle it"
    fi

    ( cd build/windows/nsis
      "$MAKENSIS_BIN" \
        -DWAILS_INSTALL_SCOPE="$INSTALL_SCOPE" \
        -DREQUEST_EXECUTION_LEVEL="$INSTALL_SCOPE" \
        -DINFO_PROJECTNAME="$APP_NAME" \
        "${NSIS_DEFINES[@]}" \
        project.nsi )
    log "installer written to build/windows/nsis/${APP_NAME}-${ARCH}-installer.exe"
  else
    warn "makensis not found; skipping installer (install NSIS, e.g. 'scoop install nsis')"
  fi
elif [[ "$TARGET" != "exe" ]]; then
  die "unknown target '$TARGET' (expected: exe | installer | all)"
fi

log "done"
