#!/bin/sh
# install.sh - Install azdo-tui from GitHub releases
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Elpulgo/azdo/main/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/Elpulgo/azdo/main/install.sh | sh -s -- --version v0.1.0
#   ./install.sh
#   ./install.sh --version v0.1.0
#   ./install.sh --install-dir /custom/path

set -e

# ─── Configuration ─────────────────────────────────────────────────────────────

REPO_OWNER="Elpulgo"
REPO_NAME="azdo"
BINARY_NAME="azdo"
CONFIG_DIR_NAME="azdo-tui"
GITHUB_API="https://api.github.com"
GITHUB_DOWNLOAD="https://github.com"

# Defaults (can be overridden by flags)
INSTALL_DIR=""
VERSION=""
SKIP_CONFIG="false"

# ─── Output helpers ────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

info() {
    printf "${BLUE}[INFO]${NC}  %s\n" "$1"
}

success() {
    printf "${GREEN}[OK]${NC}    %s\n" "$1"
}

warn() {
    printf "${YELLOW}[WARN]${NC}  %s\n" "$1"
}

error() {
    printf "${RED}[ERROR]${NC} %s\n" "$1" >&2
}

step() {
    printf "\n${CYAN}${BOLD}==> %s${NC}\n" "$1"
}

# ─── Parse arguments ──────────────────────────────────────────────────────────

parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --version|-v)
                VERSION="$2"
                shift 2
                ;;
            --install-dir|-d)
                INSTALL_DIR="$2"
                shift 2
                ;;
            --skip-config)
                SKIP_CONFIG="true"
                shift
                ;;
            --help|-h)
                usage
                exit 0
                ;;
            *)
                error "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done
}

usage() {
    cat <<EOF
Usage: install.sh [OPTIONS]

Install azdo-tui from GitHub releases.

Options:
  --version, -v VERSION     Install a specific version (e.g., v0.1.0)
                            Default: latest release
  --install-dir, -d DIR     Custom installation directory
                            Default: /usr/local/bin (or ~/.local/bin as fallback)
  --skip-config             Skip creating the configuration file
  --help, -h                Show this help message

Examples:
  # Install latest version
  curl -fsSL https://raw.githubusercontent.com/Elpulgo/azdo/main/install.sh | sh

  # Install specific version
  ./install.sh --version v0.1.0

  # Install to custom directory
  ./install.sh --install-dir ~/bin
EOF
}

# ─── OS / Architecture detection ──────────────────────────────────────────────

detect_os() {
    OS="$(uname -s)"
    case "$OS" in
        Linux*)   OS_NAME="Linux"   ;;
        Darwin*)  OS_NAME="Darwin"  ;;
        MINGW*|MSYS*|CYGWIN*)
            OS_NAME="Windows"
            ;;
        *)
            error "Unsupported operating system: $OS"
            error "azdo-tui supports Linux, macOS, and Windows."
            error "For Windows, consider using the PowerShell installer (install.ps1) instead."
            exit 1
            ;;
    esac
    success "Detected OS: $OS_NAME"
}

detect_arch() {
    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64|amd64)   ARCH_NAME="x86_64" ;;
        aarch64|arm64)  ARCH_NAME="arm64"   ;;
        *)
            error "Unsupported architecture: $ARCH"
            error "azdo-tui supports x86_64 (amd64) and arm64 (aarch64)."
            exit 1
            ;;
    esac
    success "Detected architecture: $ARCH_NAME"
}

# ─── Dependency checks ────────────────────────────────────────────────────────

check_dependencies() {
    # We need either curl or wget for downloading
    if command -v curl >/dev/null 2>&1; then
        DOWNLOADER="curl"
    elif command -v wget >/dev/null 2>&1; then
        DOWNLOADER="wget"
    else
        error "Neither 'curl' nor 'wget' found."
        error "Please install one of them and try again:"
        error "  - Ubuntu/Debian: sudo apt install curl"
        error "  - Fedora/RHEL:   sudo dnf install curl"
        error "  - macOS:         curl is pre-installed"
        exit 1
    fi
    success "Using downloader: $DOWNLOADER"

    # We need tar for extracting (unless Windows .zip)
    if [ "$OS_NAME" != "Windows" ]; then
        if ! command -v tar >/dev/null 2>&1; then
            error "'tar' is required for extracting the archive but was not found."
            error "Please install tar and try again."
            exit 1
        fi
    fi

    # sha256sum or shasum for checksum verification
    if command -v sha256sum >/dev/null 2>&1; then
        SHA_CMD="sha256sum"
    elif command -v shasum >/dev/null 2>&1; then
        SHA_CMD="shasum -a 256"
    else
        warn "Neither 'sha256sum' nor 'shasum' found. Skipping checksum verification."
        SHA_CMD=""
    fi
}

# ─── Download helper ──────────────────────────────────────────────────────────

download() {
    url="$1"
    output="$2"

    if [ "$DOWNLOADER" = "curl" ]; then
        curl -fsSL -o "$output" "$url"
    else
        wget -q -O "$output" "$url"
    fi
}

download_to_stdout() {
    url="$1"

    if [ "$DOWNLOADER" = "curl" ]; then
        curl -fsSL "$url"
    else
        wget -q -O - "$url"
    fi
}

# ─── Version resolution ───────────────────────────────────────────────────────

resolve_version() {
    if [ -n "$VERSION" ]; then
        # Ensure version starts with 'v'
        case "$VERSION" in
            v*) ;;
            *)  VERSION="v$VERSION" ;;
        esac
        success "Using specified version: $VERSION"
        return
    fi

    info "Fetching latest release version..."

    LATEST_URL="$GITHUB_API/repos/$REPO_OWNER/$REPO_NAME/releases/latest"
    RESPONSE=$(download_to_stdout "$LATEST_URL" 2>&1) || {
        error "Failed to fetch latest release from GitHub."
        error "This could mean:"
        error "  - No releases have been published yet"
        error "  - GitHub API rate limit exceeded (try again later)"
        error "  - Network connectivity issues"
        error ""
        error "You can specify a version manually: ./install.sh --version v0.1.0"
        error "Check available releases at: $GITHUB_DOWNLOAD/$REPO_OWNER/$REPO_NAME/releases"
        exit 1
    }

    # Extract tag_name from JSON response (works without jq)
    VERSION=$(printf '%s' "$RESPONSE" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | grep -o '"[^"]*"$' | tr -d '"')

    if [ -z "$VERSION" ]; then
        error "Could not determine the latest version."
        error "The GitHub API response may have changed or no releases exist."
        error ""
        error "You can specify a version manually: ./install.sh --version v0.1.0"
        error "Check available releases at: $GITHUB_DOWNLOAD/$REPO_OWNER/$REPO_NAME/releases"
        exit 1
    fi

    success "Latest version: $VERSION"
}

# ─── Install directory resolution ─────────────────────────────────────────────

resolve_install_dir() {
    if [ -n "$INSTALL_DIR" ]; then
        success "Using custom install directory: $INSTALL_DIR"
        return
    fi

    if [ "$OS_NAME" = "Windows" ]; then
        # On Windows (MSYS/Git Bash), install to user's local bin
        INSTALL_DIR="$HOME/.local/bin"
    elif [ "$(id -u)" = "0" ]; then
        # Running as root
        INSTALL_DIR="/usr/local/bin"
    elif [ -d "/usr/local/bin" ] && [ -w "/usr/local/bin" ]; then
        INSTALL_DIR="/usr/local/bin"
    else
        # Fallback to user-local directory
        INSTALL_DIR="$HOME/.local/bin"
        if ! echo "$PATH" | grep -q "$INSTALL_DIR"; then
            warn "$INSTALL_DIR is not in your PATH."
            warn "Add it by appending this to your shell profile (~/.bashrc, ~/.zshrc, etc.):"
            warn "  export PATH=\"\$HOME/.local/bin:\$PATH\""
        fi
    fi

    success "Install directory: $INSTALL_DIR"
}

# ─── Download and install ─────────────────────────────────────────────────────

download_and_install() {
    # Determine archive extension
    if [ "$OS_NAME" = "Windows" ]; then
        ARCHIVE_EXT="zip"
    else
        ARCHIVE_EXT="tar.gz"
    fi

    # Strip leading 'v' from version for the archive filename
    VERSION_NUM="${VERSION#v}"

    ARCHIVE_NAME="${BINARY_NAME}_${VERSION_NUM}_${OS_NAME}_${ARCH_NAME}.${ARCHIVE_EXT}"
    DOWNLOAD_URL="$GITHUB_DOWNLOAD/$REPO_OWNER/$REPO_NAME/releases/download/$VERSION/$ARCHIVE_NAME"
    CHECKSUMS_URL="$GITHUB_DOWNLOAD/$REPO_OWNER/$REPO_NAME/releases/download/$VERSION/checksums.txt"

    info "Downloading $ARCHIVE_NAME..."

    # Create temporary directory
    TMP_DIR=$(mktemp -d 2>/dev/null || mktemp -d -t 'azdo-install')
    trap 'rm -rf "$TMP_DIR"' EXIT

    download "$DOWNLOAD_URL" "$TMP_DIR/$ARCHIVE_NAME" || {
        error "Failed to download $ARCHIVE_NAME"
        error ""
        error "Possible causes:"
        error "  - Version $VERSION may not exist"
        error "  - The release may not include a build for $OS_NAME/$ARCH_NAME"
        error "  - Network connectivity issues"
        error ""
        error "Check available releases at:"
        error "  $GITHUB_DOWNLOAD/$REPO_OWNER/$REPO_NAME/releases"
        exit 1
    }

    success "Downloaded $ARCHIVE_NAME"

    # Verify checksum if possible
    if [ -n "$SHA_CMD" ]; then
        info "Verifying checksum..."
        download "$CHECKSUMS_URL" "$TMP_DIR/checksums.txt" 2>/dev/null && {
            EXPECTED=$(grep "$ARCHIVE_NAME" "$TMP_DIR/checksums.txt" | awk '{print $1}')
            if [ -n "$EXPECTED" ]; then
                ACTUAL=$(cd "$TMP_DIR" && $SHA_CMD "$ARCHIVE_NAME" | awk '{print $1}')
                if [ "$EXPECTED" = "$ACTUAL" ]; then
                    success "Checksum verified"
                else
                    error "Checksum verification failed!"
                    error "  Expected: $EXPECTED"
                    error "  Got:      $ACTUAL"
                    error "The downloaded file may be corrupted. Please try again."
                    exit 1
                fi
            else
                warn "Archive not found in checksums file. Skipping verification."
            fi
        } || {
            warn "Could not download checksums file. Skipping verification."
        }
    fi

    # Extract archive
    info "Extracting archive..."
    if [ "$ARCHIVE_EXT" = "zip" ]; then
        if command -v unzip >/dev/null 2>&1; then
            unzip -o -q "$TMP_DIR/$ARCHIVE_NAME" -d "$TMP_DIR/extracted"
        else
            error "'unzip' is required to extract the Windows archive but was not found."
            exit 1
        fi
    else
        mkdir -p "$TMP_DIR/extracted"
        tar -xzf "$TMP_DIR/$ARCHIVE_NAME" -C "$TMP_DIR/extracted"
    fi

    success "Extracted archive"

    # Find the binary in extracted files
    if [ "$OS_NAME" = "Windows" ]; then
        BINARY_FILE="$BINARY_NAME.exe"
    else
        BINARY_FILE="$BINARY_NAME"
    fi

    EXTRACTED_BINARY=$(find "$TMP_DIR/extracted" -name "$BINARY_FILE" -type f 2>/dev/null | head -1)
    if [ -z "$EXTRACTED_BINARY" ]; then
        error "Could not find '$BINARY_FILE' in the extracted archive."
        error "The archive contents may have changed. Please report this issue at:"
        error "  https://github.com/$REPO_OWNER/$REPO_NAME/issues"
        exit 1
    fi

    # Create install directory if needed
    if [ ! -d "$INSTALL_DIR" ]; then
        info "Creating install directory: $INSTALL_DIR"
        mkdir -p "$INSTALL_DIR" 2>/dev/null || {
            error "Failed to create directory: $INSTALL_DIR"
            error ""
            error "Possible fixes:"
            error "  - Run with sudo: curl ... | sudo sh"
            error "  - Use a custom directory: ./install.sh --install-dir ~/bin"
            error "  - Create the directory manually: mkdir -p $INSTALL_DIR"
            exit 1
        }
    fi

    # Install the binary
    info "Installing $BINARY_FILE to $INSTALL_DIR..."
    cp "$EXTRACTED_BINARY" "$INSTALL_DIR/$BINARY_FILE" 2>/dev/null || {
        error "Failed to copy binary to $INSTALL_DIR/$BINARY_FILE"
        error ""
        error "Possible fixes:"
        error "  - Run with sudo: curl ... | sudo sh"
        error "  - Use a custom directory: ./install.sh --install-dir ~/bin"
        error "  - Check permissions: ls -la $INSTALL_DIR"
        exit 1
    }

    chmod +x "$INSTALL_DIR/$BINARY_FILE" 2>/dev/null || true

    success "Installed $BINARY_FILE to $INSTALL_DIR/$BINARY_FILE"
}

# ─── Configuration file setup ─────────────────────────────────────────────────

setup_config() {
    if [ "$SKIP_CONFIG" = "true" ]; then
        info "Skipping configuration file setup (--skip-config)"
        return
    fi

    # Determine config directory
    if [ "$OS_NAME" = "Windows" ]; then
        CONFIG_DIR="$USERPROFILE/.config/$CONFIG_DIR_NAME"
        # Fallback to HOME if USERPROFILE is not set (e.g., in MSYS)
        if [ -z "$CONFIG_DIR" ] || [ "$CONFIG_DIR" = "/.config/$CONFIG_DIR_NAME" ]; then
            CONFIG_DIR="$HOME/.config/$CONFIG_DIR_NAME"
        fi
    else
        # Use XDG_CONFIG_HOME if set, otherwise ~/.config
        CONFIG_BASE="${XDG_CONFIG_HOME:-$HOME/.config}"
        CONFIG_DIR="$CONFIG_BASE/$CONFIG_DIR_NAME"
    fi

    CONFIG_FILE="$CONFIG_DIR/config.yaml"

    if [ -f "$CONFIG_FILE" ]; then
        success "Configuration file already exists at $CONFIG_FILE"
        info "Skipping config creation to preserve your settings."
        return
    fi

    info "Creating configuration directory: $CONFIG_DIR"
    mkdir -p "$CONFIG_DIR" 2>/dev/null || {
        error "Failed to create config directory: $CONFIG_DIR"
        error "You can create it manually later:"
        error "  mkdir -p $CONFIG_DIR"
        return
    }

    info "Creating configuration file with placeholder values..."
    cat > "$CONFIG_FILE" <<'CONFIGEOF'
# Azure DevOps TUI Configuration
# Documentation: https://github.com/Elpulgo/azdo#configuration

# Azure DevOps organization name (required)
# Replace with your organization name from https://dev.azure.com/{organization}
organization: your-org-name

# Azure DevOps project name(s) (required)
# Simple format:
projects:
  - your-project-name

# With display names (friendly name shown in UI):
#   projects:
#     - name: ugly-api-project-name
#       display_name: My Project
#     - simple-project

# Polling interval in seconds (optional, default: 60)
polling_interval: 60

# Theme (optional, default: dark)
# Available: dark, gruvbox, nord, dracula, catppuccin, github, retro
# Custom themes: Place JSON files in ~/.config/<CONFIG_DIR>/themes/
theme: dracula
CONFIGEOF

    success "Created configuration file at ~/.config/$CONFIG_DIR"
    warn "Edit this file with your Azure DevOps organization and project details before running azdo-tui."
}

# ─── Summary ───────────────────────────────────────────────────────────────────

print_summary() {
    printf "\n"
    printf "${GREEN}${BOLD}────────────────────────────────────────────────${NC}\n"
    printf "${GREEN}${BOLD}  azdo-tui %s installed successfully!${NC}\n" "$VERSION"
    printf "${GREEN}${BOLD}────────────────────────────────────────────────${NC}\n"
    printf "\n"

    if [ "$SKIP_CONFIG" != "true" ] && [ -n "$CONFIG_FILE" ] && [ -f "$CONFIG_FILE" ]; then
        printf "  ${BOLD}Next steps:${NC}\n"
        printf "    1. Edit your config file:\n"
        printf "       ${CYAN}%s${NC}\n" "$CONFIG_FILE"
        printf "    2. Set your organization and project name(s)\n"
        printf "    3. Run ${CYAN}%s${NC}\n" "$BINARY_NAME"
        printf "       (You'll be prompted for your Azure DevOps PAT on first run)\n"
    else
        printf "  ${BOLD}Next steps:${NC}\n"
        printf "    1. Create a config file at ${CYAN}~/.config/$CONFIG_DIR_NAME/config.yaml${NC}\n"
        printf "    2. Run ${CYAN}%s${NC}\n" "$BINARY_NAME"
    fi

    printf "\n"
    printf "  ${BOLD}Documentation:${NC} https://github.com/$REPO_OWNER/$REPO_NAME#readme\n"
    printf "\n"
}

# ─── Main ──────────────────────────────────────────────────────────────────────

main() {
    printf "\n${BOLD}azdo-tui installer${NC}\n"
    printf "Azure DevOps TUI for your terminal\n\n"

    parse_args "$@"

    step "Detecting system"
    detect_os
    detect_arch
    check_dependencies

    step "Resolving version"
    resolve_version
    resolve_install_dir

    step "Downloading and installing"
    download_and_install

    step "Setting up configuration"
    setup_config

    print_summary
}

main "$@"
