#!/bin/bash
#
# TunnelBypass Deployment Script
# Automatically detects remote server OS/arch and deploys the correct binary
#
# Usage:
#   ./deploy.sh [options]
#
# Examples:
#   # Deploy with password auth
#   ./deploy.sh -H server.example.com -u root -p mypassword
#
#   # Deploy with SSH key
#   ./deploy.sh -H server.example.com -u root -i ~/.ssh/id_rsa
#
#   # Deploy specific version
#   ./deploy.sh -H server.example.com -u root -i ~/.ssh/id_rsa -v v1.2.1
#
#   # Dry run (show what would happen)
#   ./deploy.sh -H server.example.com -u root -i ~/.ssh/id_rsa -n
#

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color
BOLD='\033[1m'

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

# Default values
REMOTE_HOST=""
REMOTE_PORT="22"
REMOTE_USER="root"
REMOTE_PASS=""
SSH_KEY=""
VERSION=""
RELEASE_DIR=""
DRY_RUN=false
SKIP_RESTART=false
TIMEOUT=30

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${CYAN}${BOLD}[STEP]${NC} $1"
}

# Show usage
usage() {
    cat <<EOF
TunnelBypass Deployment Script

Usage: $0 [OPTIONS]

Required:
  -H, --host HOST         Remote server hostname or IP
  -u, --user USER         Remote username (default: root)

Authentication (one required):
  -p, --password PASS     SSH password
  -i, --identity FILE     SSH private key file

Optional:
  -P, --port PORT         SSH port (default: 22)
  -v, --version VERSION   Specific version to deploy (auto-detected if not set)
  -r, --release-dir DIR   Directory containing release files (default: PROJECT_DIR/build/<version>)
  -n, --dry-run           Show what would happen without executing
  -s, --skip-restart      Don't restart services after deployment
  -t, --timeout SEC       SSH timeout in seconds (default: 30)
  -h, --help              Show this help

Examples:
  # Deploy with password
  $0 -H server.example.com -u root -p mypassword

  # Deploy with SSH key
  $0 -H server.example.com -u root -i ~/.ssh/id_rsa

  # Deploy specific version with dry-run
  $0 -H 192.168.1.100 -u admin -i ~/.ssh/id_rsa -v v1.2.1 -n

EOF
    exit 0
}

# Parse arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -H|--host)
                REMOTE_HOST="$2"
                shift 2
                ;;
            -P|--port)
                REMOTE_PORT="$2"
                shift 2
                ;;
            -u|--user)
                REMOTE_USER="$2"
                shift 2
                ;;
            -p|--password)
                REMOTE_PASS="$2"
                shift 2
                ;;
            -i|--identity)
                SSH_KEY="$2"
                shift 2
                ;;
            -v|--version)
                VERSION="$2"
                shift 2
                ;;
            -r|--release-dir)
                RELEASE_DIR="$2"
                shift 2
                ;;
            -n|--dry-run)
                DRY_RUN=true
                shift
                ;;
            -s|--skip-restart)
                SKIP_RESTART=true
                shift
                ;;
            -t|--timeout)
                TIMEOUT="$2"
                shift 2
                ;;
            -h|--help)
                usage
                ;;
            *)
                log_error "Unknown option: $1"
                usage
                exit 1
                ;;
        esac
    done

    # Validate required args
    if [[ -z "$REMOTE_HOST" ]]; then
        log_error "Host is required (-H)"
        usage
        exit 1
    fi

    if [[ -z "$REMOTE_PASS" && -z "$SSH_KEY" ]]; then
        log_error "Either password (-p) or SSH key (-i) is required"
        usage
        exit 1
    fi

    if [[ -n "$SSH_KEY" && ! -f "$SSH_KEY" ]]; then
        log_error "SSH key file not found: $SSH_KEY"
        exit 1
    fi
}

# Detect latest version (release artifacts live under build/<version>/ per build-release.sh / build-release.ps1)
detect_latest_version() {
    log_step "Detecting latest version..."

    if [[ -n "$VERSION" ]]; then
        log_info "Using specified version: $VERSION"
    else
        local latest_version=""

        if [[ -f "$PROJECT_DIR/VERSION" ]]; then
            latest_version=$(tr -d '[:space:]' < "$PROJECT_DIR/VERSION")
        fi

        if [[ -z "$latest_version" && -d "$PROJECT_DIR/build" ]]; then
            local newest="" best=0 m f d
            for d in "$PROJECT_DIR/build"/*/; do
                [[ -d "$d" ]] || continue
                for f in "$d"tunnelbypass_*_linux_amd64.tar.gz; do
                    [[ -f "$f" ]] || continue
                    m=0
                    if m=$(stat -f %m "$f" 2>/dev/null); then :; elif m=$(stat -c %Y "$f" 2>/dev/null); then :; else m=0; fi
                    if (( m > best )); then best=$m; newest=$f; fi
                done
            done
            if [[ -n "$newest" ]]; then
                latest_version=$(basename "$newest" | sed -E 's/tunnelbypass_([^_]+)_linux_amd64.tar.gz/\1/')
            fi
        fi

        if [[ -z "$latest_version" ]]; then
            log_error "Could not detect version. Please specify with -v"
            log_info "Expected artifacts under: $PROJECT_DIR/build/<version>/ (from build-release) or VERSION at repo root"
            exit 1
        fi

        VERSION="$latest_version"
        log_success "Detected version: $VERSION"
    fi

    if [[ -z "$RELEASE_DIR" ]]; then
        RELEASE_DIR="$PROJECT_DIR/build/$VERSION"
        log_info "Release directory (default): $RELEASE_DIR"
    fi
}

# Detect remote OS and architecture
detect_remote_system() {
    log_step "Detecting remote system ($REMOTE_HOST)..."

    local ssh_cmd
    if [[ -n "$SSH_KEY" ]]; then
        ssh_cmd="ssh -i $SSH_KEY -p $REMOTE_PORT -o ConnectTimeout=$TIMEOUT -o StrictHostKeyChecking=no"
    else
        # Use sshpass for password auth
        if ! command -v sshpass &> /dev/null; then
            log_error "sshpass is required for password authentication"
            log_info "Install with: apt-get install sshpass  (or equivalent)"
            exit 1
        fi
        ssh_cmd="sshpass -p '$REMOTE_PASS' ssh -p $REMOTE_PORT -o ConnectTimeout=$TIMEOUT -o StrictHostKeyChecking=no"
    fi

    log_info "Connecting to $REMOTE_USER@$REMOTE_HOST:$REMOTE_PORT..."

    # Detect OS
    local remote_os
    remote_os=$(eval "$ssh_cmd $REMOTE_USER@$REMOTE_HOST 'uname -s'" 2>/dev/null) || {
        log_error "Failed to connect to remote server"
        log_info "Check: host reachable, credentials correct, SSH enabled"
        exit 1
    }

    # Detect architecture
    local remote_arch
    remote_arch=$(eval "$ssh_cmd $REMOTE_USER@$REMOTE_HOST 'uname -m'")

    log_info "Remote OS: $remote_os"
    log_info "Remote Architecture: $remote_arch"

    # Map to release naming convention
    local mapped_os=""
    local mapped_arch=""

    case "$remote_os" in
        Linux)
            mapped_os="linux"
            ;;
        Darwin)
            log_error "Remote macOS is not supported as a TunnelBypass server host; use Linux or Windows."
            exit 1
            ;;
        CYGWIN*|MINGW*|MSYS*)
            mapped_os="windows"
            ;;
        *)
            log_error "Unsupported OS: $remote_os"
            exit 1
            ;;
    esac

    case "$remote_arch" in
        x86_64|amd64)
            mapped_arch="amd64"
            ;;
        aarch64|arm64)
            mapped_arch="arm64"
            ;;
        armv7l)
            mapped_arch="arm"
            ;;
        i386|i686)
            mapped_arch="386"
            ;;
        *)
            log_error "Unsupported architecture: $remote_arch"
            exit 1
            ;;
    esac

    REMOTE_OS="$mapped_os"
    REMOTE_ARCH="$mapped_arch"

    log_success "Mapped to: ${REMOTE_OS}_${REMOTE_ARCH}"
}

# Select and validate release file
select_release_file() {
    log_step "Selecting release file..."

    local filename

    if [[ "$REMOTE_OS" == "windows" ]]; then
        filename="tunnelbypass_${VERSION}_${REMOTE_OS}_${REMOTE_ARCH}.exe"
    else
        filename="tunnelbypass_${VERSION}_${REMOTE_OS}_${REMOTE_ARCH}.tar.gz"
    fi

    RELEASE_FILE="$RELEASE_DIR/$filename"

    log_info "Looking for: $filename"

    if [[ ! -f "$RELEASE_FILE" ]]; then
        log_error "Release file not found: $RELEASE_FILE"
        log_info "Available files:"
        ls -1 "$RELEASE_DIR"/tunnelbypass_* 2>/dev/null || echo "  (none found)"
        exit 1
    fi

    log_success "Found: $filename ($(du -h "$RELEASE_FILE" | cut -f1))"
}

# Upload file to remote server
upload_file() {
    log_step "Uploading to remote server..."

    local dest_dir="/root/tunnelbypass"
    local filename=$(basename "$RELEASE_FILE")

    log_info "Destination: $REMOTE_USER@$REMOTE_HOST:$dest_dir/"

    if [[ "$DRY_RUN" == true ]]; then
        log_warn "[DRY-RUN] Would upload: $RELEASE_FILE -> $dest_dir/"
        return 0
    fi

    # Create destination directory
    local ssh_cmd_base
    if [[ -n "$SSH_KEY" ]]; then
        ssh_cmd_base="ssh -i $SSH_KEY -p $REMOTE_PORT -o StrictHostKeyChecking=no"
    else
        ssh_cmd_base="sshpass -p '$REMOTE_PASS' ssh -p $REMOTE_PORT -o StrictHostKeyChecking=no"
    fi

    eval "$ssh_cmd_base $REMOTE_USER@$REMOTE_HOST 'mkdir -p $dest_dir'"

    # Upload file
    local scp_cmd
    if [[ -n "$SSH_KEY" ]]; then
        scp_cmd="scp -i $SSH_KEY -P $REMOTE_PORT -o StrictHostKeyChecking=no"
    else
        scp_cmd="sshpass -p '$REMOTE_PASS' scp -P $REMOTE_PORT -o StrictHostKeyChecking=no"
    fi

    log_info "Uploading $filename..."
    eval "$scp_cmd '$RELEASE_FILE' $REMOTE_USER@$REMOTE_HOST:$dest_dir/"

    log_success "Upload complete"
}

# Install binary on remote server
install_remote() {
    log_step "Installing on remote server..."

    if [[ "$DRY_RUN" == true ]]; then
        log_warn "[DRY-RUN] Would stop TunnelBypass services, replace binary, restart later"
        return 0
    fi

    local ssh_cmd
    if [[ -n "$SSH_KEY" ]]; then
        ssh_cmd="ssh -i $SSH_KEY -p $REMOTE_PORT -o StrictHostKeyChecking=no $REMOTE_USER@$REMOTE_HOST"
    else
        ssh_cmd="sshpass -p '$REMOTE_PASS' ssh -p $REMOTE_PORT -o StrictHostKeyChecking=no $REMOTE_USER@$REMOTE_HOST"
    fi

    local dest_dir="/root/tunnelbypass"
    local filename=$(basename "$RELEASE_FILE")

    local stop_units='for u in TunnelBypass-SSH-Forwarder TunnelBypass-SSH TunnelBypass-WSS TunnelBypass-SSL TunnelBypass-VLESS-WS TunnelBypass-VLESS TunnelBypass-Hysteria TunnelBypass-WireGuard TunnelBypass-Tunnel TunnelBypass-UDP TunnelBypass-UDPGW; do systemctl stop "$u" 2>/dev/null || true; done'

    local install_cmds=""
    if [[ "$REMOTE_OS" == "linux" ]]; then
        log_info "Stopping TunnelBypass services (if any) before replacing binary..."
        install_cmds="$stop_units && sleep 1"
    fi

    install_cmds="$install_cmds && cd $dest_dir && rm -f tunnelbypass"

    if [[ "$filename" == *.tar.gz ]]; then
        install_cmds="$install_cmds && tar -xzf $filename"
    fi

    install_cmds="$install_cmds && chmod +x tunnelbypass"
    install_cmds="$install_cmds && install -m 0755 tunnelbypass /usr/local/bin/tunnelbypass"
    install_cmds="$install_cmds && tunnelbypass -version"

    # Trim leading " && " if no stop block (non-linux)
    install_cmds="${install_cmds# && }"

    log_info "Running installation commands..."
    eval "$ssh_cmd '$install_cmds'"

    log_success "Installation complete"
}

# Restart services
restart_services() {
    if [[ "$SKIP_RESTART" == true ]]; then
        log_warn "Skipping service restart (-s flag)"
        return 0
    fi

    if [[ "$REMOTE_OS" == "windows" ]]; then
        log_warn "Windows detected, skipping systemd service restart"
        return 0
    fi

    log_step "Restarting services..."

    if [[ "$DRY_RUN" == true ]]; then
        log_warn "[DRY-RUN] Would restart services"
        return 0
    fi

    local ssh_cmd
    if [[ -n "$SSH_KEY" ]]; then
        ssh_cmd="ssh -i $SSH_KEY -p $REMOTE_PORT -o StrictHostKeyChecking=no $REMOTE_USER@$REMOTE_HOST"
    else
        ssh_cmd="sshpass -p '$REMOTE_PASS' ssh -p $REMOTE_PORT -o StrictHostKeyChecking=no $REMOTE_USER@$REMOTE_HOST"
    fi

    # Restart services (ignore errors if services don't exist)
    local restart_cmds="systemctl daemon-reload"
    restart_cmds="$restart_cmds && systemctl restart TunnelBypass-UDPGW 2>/dev/null || true"
    restart_cmds="$restart_cmds && systemctl restart TunnelBypass-SSH 2>/dev/null || true"
    restart_cmds="$restart_cmds && systemctl restart TunnelBypass-SSH-Forwarder 2>/dev/null || true"
    restart_cmds="$restart_cmds && systemctl restart TunnelBypass-SSL 2>/dev/null || true"
    restart_cmds="$restart_cmds && systemctl restart TunnelBypass-WSS 2>/dev/null || true"
    restart_cmds="$restart_cmds && systemctl restart TunnelBypass-VLESS-WS 2>/dev/null || true"
    restart_cmds="$restart_cmds && systemctl restart TunnelBypass-VLESS 2>/dev/null || true"
    restart_cmds="$restart_cmds && systemctl restart TunnelBypass-Hysteria 2>/dev/null || true"
    restart_cmds="$restart_cmds && systemctl restart TunnelBypass-WireGuard 2>/dev/null || true"
    restart_cmds="$restart_cmds && systemctl restart TunnelBypass-Tunnel 2>/dev/null || true"
    restart_cmds="$restart_cmds && systemctl restart TunnelBypass-UDP 2>/dev/null || true"

    log_info "Restarting TunnelBypass services..."
    eval "$ssh_cmd '$restart_cmds'" || {
        log_warn "Some services may not have restarted (this is OK if not previously installed)"
    }

    log_success "Services restarted"
}

# Validate installation
validate_installation() {
    log_step "Validating installation..."

    if [[ "$DRY_RUN" == true ]]; then
        log_warn "[DRY-RUN] Would validate installation"
        return 0
    fi

    local ssh_cmd
    if [[ -n "$SSH_KEY" ]]; then
        ssh_cmd="ssh -i $SSH_KEY -p $REMOTE_PORT -o StrictHostKeyChecking=no $REMOTE_USER@$REMOTE_HOST"
    else
        ssh_cmd="sshpass -p '$REMOTE_PASS' ssh -p $REMOTE_PORT -o StrictHostKeyChecking=no $REMOTE_USER@$REMOTE_HOST"
    fi

    log_info "Checking tunnelbypass version..."
    local version_output
    version_output=$(eval "$ssh_cmd 'tunnelbypass -version 2>&1 || echo \"VERSION_CHECK_FAILED\"'")

    if [[ "$version_output" == *"VERSION_CHECK_FAILED"* ]]; then
        log_error "Installation validation failed"
        exit 1
    fi

    log_success "Validation successful: $version_output"

    # Check service status if Linux
    if [[ "$REMOTE_OS" == "linux" && "$SKIP_RESTART" == false ]]; then
        log_info "Checking service status..."
        eval "$ssh_cmd 'systemctl is-active TunnelBypass-SSH 2>/dev/null && echo \"SSH service: ACTIVE\" || echo \"SSH service: not running (may need wizard setup)\"'" || true
    fi
}

# Print summary
print_summary() {
    echo ""
    echo "========================================"
    echo -e "${GREEN}Deployment Complete!${NC}"
    echo "========================================"
    echo ""
    echo "Server: $REMOTE_HOST:$REMOTE_PORT"
    echo "User: $REMOTE_USER"
    echo "OS/Arch: ${REMOTE_OS}_${REMOTE_ARCH}"
    echo "Version: $VERSION"
    echo "Binary: /usr/local/bin/tunnelbypass"
    echo ""
    echo "Next steps:"
    echo "  1. SSH into server: ssh $REMOTE_USER@$REMOTE_HOST"
    echo "  2. Run wizard: sudo tunnelbypass wizard"
    echo "  3. Select WSS transport and complete setup"
    echo ""
    echo "Check service status:"
    echo "  systemctl status TunnelBypass-SSH"
    echo "  journalctl -u TunnelBypass-SSH -f"
    echo ""
}

# Main function
main() {
    echo -e "${BOLD}"
    echo "╔══════════════════════════════════════════════════════════════╗"
    echo "║          TunnelBypass Deployment Script                      ║"
    echo "╚══════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"

    parse_args "$@"

    if [[ "$DRY_RUN" == true ]]; then
        log_warn "DRY-RUN MODE: No changes will be made"
        echo ""
    fi

    # Detect version
    detect_latest_version

    # Detect remote system
    detect_remote_system

    # Select release file
    select_release_file

    # Upload file
    upload_file

    # Install on remote
    install_remote

    # Restart services
    restart_services

    # Validate
    validate_installation

    # Print summary
    print_summary
}

# Run main
main "$@"
