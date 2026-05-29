#!/bin/bash
# install.sh — install oramalama to your PATH
#
# Default: downloads the pre-built Go binary from GitHub Releases.
# Use --legacy to install the original bash script instead.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="${HOME}/.local/bin"
BASH_COMP_DIR="${HOME}/.local/share/bash-completion/completions"
ZSH_COMP_DIR="${HOME}/.local/share/zsh/site-functions"
UNINSTALL=false
LEGACY=false
VERSION="latest"

usage() {
    cat <<EOF
Usage: $0 [options]

Options:
  --prefix <dir>   Install to <dir>/bin instead of ~/.local/bin
  --version <tag>  Install a specific release version (default: latest)
  --legacy         Install the legacy bash script instead of the Go binary
  --uninstall      Remove installed files
  --help           Show this help

Examples:
  $0                          # install Go binary to ~/.local/bin
  $0 --legacy                 # install bash script (needs gum + jq)
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
        --version)   VERSION="$2"; shift 2 ;;
        --legacy)    LEGACY=true; shift ;;
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

# ── Detect OS/Arch for binary download ─────────────────────────────────────
detect_platform() {
    local os arch

    case "$(uname -s)" in
        Linux)  os="linux" ;;
        Darwin) os="darwin" ;;
        *)      echo "Unsupported OS: $(uname -s)"; return 1 ;;
    esac

    case "$(uname -m)" in
        x86_64|amd64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *)            echo "Unsupported arch: $(uname -m)"; return 1 ;;
    esac

    echo "${os}_${arch}"
}

if $LEGACY; then
    # ── Legacy bash install ────────────────────────────────────────────────
    echo "Installing legacy bash version…"

    # Dependency check (bash-specific deps)
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
    fi

    mkdir -p "$INSTALL_DIR"
    install -m 755 "${SCRIPT_DIR}/legacy/oramalama.bash" "${INSTALL_DIR}/oramalama"
    install -m 755 "${SCRIPT_DIR}/opencode-rl"           "${INSTALL_DIR}/opencode-rl"
    echo "✅ ${INSTALL_DIR}/oramalama (bash legacy)"
    echo "✅ ${INSTALL_DIR}/opencode-rl"

else
    # ── Go binary install (default) ───────────────────────────────────────
    local_install() {
        echo "Building from local source…"
        if ! command -v go &>/dev/null; then
            echo "Error: Go is required to build from source."
            echo "Install Go from https://go.dev/dl/ or use --legacy for bash."
            exit 1
        fi
        mkdir -p "$INSTALL_DIR"
        (cd "${SCRIPT_DIR}" && go build -o "${INSTALL_DIR}/oramalama" ./cmd/oramalama-go)
        echo "✅ ${INSTALL_DIR}/oramalama (built from source)"
    }

    download_release() {
        local platform="$1"
        local tag="${2}"
        local url

        if [ "$tag" = "latest" ]; then
            url="https://github.com/hanthor/oramalama/releases/latest/download/oramalama_${platform}.tar.gz"
        else
            url="https://github.com/hanthor/oramalama/releases/download/${tag}/oramalama_${platform}.tar.gz"
        fi

        echo "Downloading oramalama ${tag} for ${platform}…"
        local tmpdir
        tmpdir="$(mktemp -d)"
        trap 'rm -rf "$tmpdir"' EXIT

        if ! curl -fsSL "$url" -o "${tmpdir}/oramalama.tar.gz"; then
            echo "⚠️  Could not download binary (platform: ${platform}, tag: ${tag})."
            echo "   Falling back to local source build."
            rm -rf "$tmpdir"
            trap - EXIT
            return 1
        fi

        mkdir -p "$INSTALL_DIR"
        tar xzf "${tmpdir}/oramalama.tar.gz" -C "$tmpdir"
        install -m 755 "${tmpdir}/oramalama" "${INSTALL_DIR}/oramalama"
        echo "✅ ${INSTALL_DIR}/oramalama (downloaded)"
        echo "✅ ${INSTALL_DIR}/opencode-rl (see below)"

        trap - EXIT
        return 0
    }

    platform="$(detect_platform)"
    if ! download_release "$platform" "$VERSION"; then
        local_install
    fi

    # Create opencode-rl wrapper
    cat > "${INSTALL_DIR}/opencode-rl" <<'WRAPPER'
#!/bin/bash
# opencode-rl: shortcut for "oramalama launch --tool opencode"
exec oramalama launch --tool opencode "$@"
WRAPPER
    chmod +x "${INSTALL_DIR}/opencode-rl"
    echo "✅ ${INSTALL_DIR}/opencode-rl"

    # Check required deps (Go binary deps only)
    missing=()
    for dep in ramalama curl; do
        command -v "$dep" &>/dev/null || missing+=("$dep")
    done
    if [ ${#missing[@]} -gt 0 ]; then
        echo "⚠️  Missing required dependencies: ${missing[*]}"
        for dep in "${missing[@]}"; do
            case "$dep" in
                ramalama) echo "   ramalama  → https://github.com/containers/ramalama" ;;
                curl)     echo "   curl      → install via your system package manager" ;;
            esac
        done
    fi
fi

# ── Install opencode-rl (always) ─────────────────────────────────────────────
if [ ! -f "${INSTALL_DIR}/opencode-rl" ]; then
    install -m 755 "${SCRIPT_DIR}/opencode-rl" "${INSTALL_DIR}/opencode-rl"
    echo "✅ ${INSTALL_DIR}/opencode-rl"
fi

# ── Shell completions ────────────────────────────────────────────────────────
mkdir -p "$BASH_COMP_DIR" "$ZSH_COMP_DIR"

# Bash: install pre-built if available, otherwise generate
if [ -f "${SCRIPT_DIR}/completions/oramalama.bash" ]; then
    install -m 644 "${SCRIPT_DIR}/completions/oramalama.bash" "${BASH_COMP_DIR}/oramalama"
    echo "✅ Bash completion → ${BASH_COMP_DIR}/oramalama"
fi

# Zsh: install pre-built if available, otherwise generate
if [ -f "${SCRIPT_DIR}/completions/oramalama.zsh" ]; then
    install -m 644 "${SCRIPT_DIR}/completions/oramalama.zsh" "${ZSH_COMP_DIR}/_oramalama"
    echo "✅ Zsh completion  → ${ZSH_COMP_DIR}/_oramalama"
fi

# ── PATH reminder ────────────────────────────────────────────────────────────
if ! echo ":${PATH}:" | grep -q ":${INSTALL_DIR}:"; then
    echo ""
    echo "⚠️  ${INSTALL_DIR} is not in your PATH. Add to ~/.bashrc / ~/.zshrc:"
    echo "   export PATH=\"\${HOME}/.local/bin:\${PATH}\""
fi

echo ""
echo "🎉 Done!  Run: oramalama --help"
