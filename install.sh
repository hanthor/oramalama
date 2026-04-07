#!/bin/bash
# install.sh — install oramalama to your PATH with shell completions
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="${HOME}/.local/bin"
BASH_COMP_DIR="${HOME}/.local/share/bash-completion/completions"
ZSH_COMP_DIR="${HOME}/.local/share/zsh/site-functions"
UNINSTALL=false

usage() {
    cat <<EOF
Usage: $0 [options]

Options:
  --prefix <dir>   Install to <dir>/bin instead of ~/.local/bin
  --uninstall      Remove installed files
  --help           Show this help

Examples:
  $0                          # install to ~/.local/bin
  $0 --prefix /usr/local      # install system-wide (may need sudo)
  sudo $0 --prefix /usr/local
EOF
    exit 0
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --prefix)    INSTALL_DIR="${2}/bin"
                     BASH_COMP_DIR="${2}/share/bash-completion/completions"
                     ZSH_COMP_DIR="${2}/share/zsh/site-functions"
                     shift 2 ;;
        --uninstall) UNINSTALL=true; shift ;;
        --help|-h)   usage ;;
        *)           echo "Unknown option: $1"; usage ;;
    esac
done

if $UNINSTALL; then
    echo "Removing oramalama…"
    rm -f "${INSTALL_DIR}/oramalama" \
          "${INSTALL_DIR}/opencode-rl" \
          "${BASH_COMP_DIR}/oramalama" \
          "${ZSH_COMP_DIR}/_oramalama"
    echo "Done."
    exit 0
fi

# ── Dependency check ────────────────────────────────────────────────────────
echo "Checking dependencies…"
missing=()
for dep in ramalama gum jq curl; do
    command -v "$dep" &>/dev/null || missing+=("$dep")
done

if [ ${#missing[@]} -gt 0 ]; then
    echo "⚠️  Missing required dependencies: ${missing[*]}"
    for dep in "${missing[@]}"; do
        case "$dep" in
            ramalama) echo "   ramalama  → https://github.com/containers/ramalama" ;;
            gum)      echo "   gum       → https://github.com/charmbracelet/gum" ;;
            jq)       echo "   jq        → https://jqlang.github.io/jq/download/" ;;
            curl)     echo "   curl      → install via your system package manager" ;;
        esac
    done
    echo "   Install them before running oramalama."
fi

command -v llmfit &>/dev/null \
    || echo "ℹ️   llmfit not found (optional — enables 'oramalama search')"
command -v opencode &>/dev/null \
    || echo "ℹ️   opencode not found (optional — enables 'oramalama launch --tool opencode')"
command -v goose &>/dev/null \
    || echo "ℹ️   goose not found (optional — enables 'oramalama launch --tool goose')"

# ── Install binaries ────────────────────────────────────────────────────────
mkdir -p "$INSTALL_DIR"
install -m 755 "${SCRIPT_DIR}/oramalama"   "${INSTALL_DIR}/oramalama"
install -m 755 "${SCRIPT_DIR}/opencode-rl" "${INSTALL_DIR}/opencode-rl"
echo "✅ ${INSTALL_DIR}/oramalama"
echo "✅ ${INSTALL_DIR}/opencode-rl"

# ── Bash completion ─────────────────────────────────────────────────────────
mkdir -p "$BASH_COMP_DIR"
install -m 644 "${SCRIPT_DIR}/completions/oramalama.bash" "${BASH_COMP_DIR}/oramalama"
echo "✅ Bash completion → ${BASH_COMP_DIR}/oramalama"

# ── Zsh completion ──────────────────────────────────────────────────────────
mkdir -p "$ZSH_COMP_DIR"
install -m 644 "${SCRIPT_DIR}/completions/oramalama.zsh" "${ZSH_COMP_DIR}/_oramalama"
echo "✅ Zsh completion  → ${ZSH_COMP_DIR}/_oramalama"

# ── PATH reminder ───────────────────────────────────────────────────────────
if ! echo ":${PATH}:" | grep -q ":${INSTALL_DIR}:"; then
    echo ""
    echo "⚠️  ${INSTALL_DIR} is not in your PATH. Add to ~/.bashrc / ~/.zshrc:"
    echo "   export PATH=\"\${HOME}/.local/bin:\${PATH}\""
fi

echo ""
echo "🎉 Done!  Run: oramalama --help"
