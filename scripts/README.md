# TunnelBypass Deployment Scripts

## Install from GitHub (local machine)

- **Linux:** `curl -fsSL https://raw.githubusercontent.com/abdelrahman30x/TunnelBypass/main/scripts/install.sh | bash`
- **Windows (PowerShell):** `irm https://raw.githubusercontent.com/abdelrahman30x/TunnelBypass/main/scripts/install.ps1 | iex`

See the root [README.md](../README.md) **Direct download** section for `INSTALL_PREFIX` / `INSTALL_VERSION` and asset naming.

---

Automated deployment scripts for TunnelBypass that detect remote server OS/architecture and deploy the correct binary.

## Features

- ✅ **Auto-detect** remote OS (Linux, Windows) and architecture (amd64, arm64, arm)
- ✅ **Select correct binary** from release files automatically
- ✅ **Support password and SSH key** authentication
- ✅ **Upload, install, and restart services** in one command
- ✅ **Validate installation** after deployment
- ✅ **Dry-run mode** to preview changes
- ✅ **Cross-platform** - run the deploy script from Linux, Windows PowerShell, or WSL

## Quick Start

### Prerequisites

**Linux (or WSL):**
```bash
# Install sshpass for password authentication (optional)
# Ubuntu/Debian:
sudo apt-get install sshpass

# CentOS/RHEL:
sudo yum install sshpass
```

**Windows:**
- PowerShell 5.0 or later
- OpenSSH client (built into Windows 10/11)
- Or use WSL with the Linux script

### Deploy with SSH Key (Recommended)

```bash
# Linux / WSL / Git Bash
./deploy.sh -H server.example.com -u root -i ~/.ssh/id_rsa

# Windows PowerShell
.\deploy.ps1 -Server server.example.com -User root -KeyFile C:\Users\Me\.ssh\id_rsa
```

### Deploy with Password

```bash
# Linux / WSL / Git Bash
./deploy.sh -H 192.168.1.100 -u admin -p mypassword

# Windows PowerShell (interactive password recommended)
.\deploy.ps1 -Server 192.168.1.100 -User admin -Password (Read-Host -AsSecureString)
```

## Usage

### Bash script (deploy.sh)

```
Usage: ./deploy.sh [OPTIONS]

Required:
  -H, --host HOST         Remote server hostname or IP
  -u, --user USER         Remote username (default: root)

Authentication (one required):
  -p, --password PASS     SSH password
  -i, --identity FILE     SSH private key file

Optional:
  -P, --port PORT         SSH port (default: 22)
  -v, --version VERSION   Specific version to deploy
  -r, --release-dir DIR   Directory with release files
  -n, --dry-run           Show what would happen
  -s, --skip-restart      Don't restart services
  -t, --timeout SEC       SSH timeout (default: 30)
  -h, --help              Show help
```

### Windows PowerShell Script (deploy.ps1)

```powershell
# Parameters
-Server      # Server hostname/IP (required) - Note: -Host is reserved in PowerShell
-Port        # SSH port (default: 22)
-User        # Username (default: root)
-Password    # Password (use Read-Host -AsSecureString for security)
-KeyFile     # SSH private key path
-Version     # Specific version
-ReleaseDir  # Release files directory
-DryRun      # Preview mode
-SkipRestart # Skip service restart
-Timeout     # SSH timeout (default: 30)
```

## Examples

### Basic Deployments

```bash
# Deploy to Ubuntu server with SSH key
./deploy.sh -H ubuntu-server.local -u ubuntu -i ~/.ssh/id_rsa

# Deploy to specific port
./deploy.sh -H server.com -P 2222 -u root -p secret123

# Deploy ARM64 build to Raspberry Pi
./deploy.sh -H 192.168.1.50 -u pi -p raspberry
```

### Advanced Usage

```bash
# Dry run to see what would happen
./deploy.sh -H prod.example.com -u root -i ~/.ssh/id_rsa -n

# Deploy specific version
./deploy.sh -H prod.example.com -u root -i ~/.ssh/id_rsa -v v1.2.3

# Use custom release directory
./deploy.sh -H prod.example.com -u root -i ~/.ssh/id_rsa -r ./releases

# Deploy without restarting services
./deploy.sh -H staging.example.com -u root -i ~/.ssh/id_rsa -s
```

### Windows Examples

```powershell
# Basic deployment with key
.\deploy.ps1 -Host server.com -User admin -KeyFile ~/.ssh/id_rsa

# With secure password prompt
$pass = Read-Host -AsSecureString "Enter password"
.\deploy.ps1 -Host server.com -User admin -Password $pass

# Dry run
.\deploy.ps1 -Host prod.example.com -User root -KeyFile ~/.ssh/id_rsa -DryRun
```

## How It Works

### Detection Flow

```
1. Connect via SSH
   ↓
2. Run: uname -s (expects Linux or Windows family on the server)
   Run: uname -m (gets Arch: x86_64, aarch64)
   ↓
3. Map to release naming:
   Linux + x86_64   → linux_amd64
   Linux + aarch64  → linux_arm64
   (Remote macOS is rejected — use a Linux or Windows server.)
   ↓
4. Select file: tunnelbypass_v1.2.1_linux_amd64.tar.gz
   ↓
5. Upload via SCP
   ↓
6. Extract, install to /usr/local/bin
   ↓
7. Restart services
   ↓
8. Validate: tunnelbypass version
```

### Architecture Mapping

| Remote `uname -m` | Release Suffix |
|-------------------|----------------|
| x86_64, amd64     | amd64          |
| aarch64, arm64    | arm64          |
| armv7l            | arm            |
| i386, i686        | 386            |

### OS Mapping

| Remote `uname -s` | Release Prefix |
|-------------------|----------------|
| Linux             | linux          |
| CYGWIN, MINGW     | windows        |

## Release File Structure

The script expects release files in this format:

```
releases/
├── tunnelbypass_v1.2.1_linux_amd64.tar.gz
├── tunnelbypass_v1.2.1_linux_arm64.tar.gz
├── tunnelbypass_v1.2.1_linux_arm.tar.gz
└── tunnelbypass_v1.2.1_windows_amd64.exe
```

### Auto-Detection

The script detects version in this priority:
1. Version specified with `-v` flag
2. `VERSION` file in release directory
3. Parse from existing release filenames

## Remote Installation Steps

The script performs these steps on the remote server:

1. **Create directory**: `mkdir -p /root/tunnelbypass`
2. **Upload file**: SCP to `/root/tunnelbypass/`
3. **Extract** (if tarball): `tar -xzf file.tar.gz`
4. **Set permissions**: `chmod +x tunnelbypass`
5. **Move to PATH**: `mv tunnelbypass /usr/local/bin/`
6. **Validate**: `tunnelbypass version`
7. **Restart services** (Linux only):
   - `systemctl daemon-reload`
   - `systemctl restart TunnelBypass-SSH`
   - `systemctl restart TunnelBypass-WSS`
   - `systemctl restart TunnelBypass-UDPGW`

## Troubleshooting

### Connection Issues

```bash
# Test SSH connection manually
ssh -i ~/.ssh/id_rsa -p 22 user@server "uname -a"

# Check if sshpass is installed
which sshpass

# Windows: Test with PowerShell
Test-NetConnection -ComputerName server -Port 22
```

### Permission Denied

```bash
# Ensure key has correct permissions
chmod 600 ~/.ssh/id_rsa

# Try password auth to verify credentials
./deploy.sh -H server -u root -p mypassword
```

### Version Not Found

```bash
# List available versions
ls -la releases/

# Specify version explicitly
./deploy.sh -H server -u root -i ~/.ssh/id_rsa -v v1.2.1
```

### Unsupported OS/Architecture

The script supports:
- **Remote OS**: Linux, Windows (remote macOS servers are not supported)
- **Arch**: x86_64/amd64, aarch64/arm64, armv7l, i386/i686

For unsupported systems, manually build and deploy.

### Service Restart Failures

This is normal if services weren't previously installed. The script will:
1. Ignore errors from missing services
2. Continue with deployment
3. Report success if binary installed correctly

After first deployment, run the wizard to set up services:
```bash
ssh root@server
sudo tunnelbypass wizard
```

## Security Notes

### SSH Keys (Recommended)

- Use SSH keys instead of passwords when possible
- Protect private keys with strong passphrases
- Use `ssh-agent` for key management

### Passwords

- Avoid passing passwords on command line (visible in history)
- Use environment variables or secure prompts:
  ```bash
  read -s PASSWORD
  ./deploy.sh -H server -u root -p "$PASSWORD"
  ```

### Network

- Use VPN or bastion hosts for production deployments
- Consider using `ProxyJump` for multi-hop SSH:
  ```bash
  ssh -J bastion@jump-host root@target-server
  ```

## CI/CD Integration

### GitHub Actions

```yaml
name: Deploy
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Setup SSH
        uses: webfactory/ssh-agent@v0.7.0
        with:
          ssh-private-key: ${{ secrets.SSH_KEY }}
      
      - name: Deploy to production
        run: |
          ./scripts/deploy.sh \
            -H ${{ secrets.PROD_HOST }} \
            -u root \
            -i ~/.ssh/id_rsa \
            -v ${{ github.ref_name }}
```

### GitLab CI

```yaml
deploy:
  stage: deploy
  script:
    - eval $(ssh-agent -s)
    - echo "$SSH_KEY" | tr -d '\r' | ssh-add -
    - ./scripts/deploy.sh -H $PROD_HOST -u root -i ~/.ssh/id_rsa
  only:
    - main
```

## Development

### Testing Dry-Run

Always test with dry-run first:
```bash
./deploy.sh -H test-server -u root -i ~/.ssh/id_rsa -n
```

### Adding New Platforms

Edit the mapping functions in the script:

```bash
# In detect_remote_system()
case "$remote_arch" in
    x86_64|amd64) mapped_arch="amd64" ;;
    aarch64|arm64) mapped_arch="arm64" ;;
    # Add new arch here
    riscv64) mapped_arch="riscv64" ;;
    *)
        log_error "Unsupported: $remote_arch"
        exit 1
        ;;
esac
```

## License

Same as TunnelBypass project.

## Support

For issues or questions:
1. Check Troubleshooting section above
2. Run with `-n` flag to see what would happen
3. Test SSH connectivity manually
4. Open an issue with output logs
