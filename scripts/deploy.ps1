#
# TunnelBypass Deployment Script for Windows
# PowerShell version of the deployment script
#
# Usage:
#   .\deploy.ps1 -Server server.example.com -User root -KeyFile C:\Users\Me\.ssh\id_rsa
#   .\deploy.ps1 -Server 192.168.1.100 -User admin -Password mypass -Version v1.2.1
#
# Note: Use -Server (not -Host) because Host is a reserved variable in PowerShell
#

[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)]
    [Alias("Host")]  # Keep Host as alias for backward compatibility
    [string]$Server,

    [int]$Port = 22,

    [Alias("UserName")]
    [string]$User = "root",

    [string]$Password = "",

    [string]$KeyFile = "",

    [string]$Version = "",

    [string]$ReleaseDir = "",

    [switch]$DryRun,

    [switch]$SkipRestart,

    [int]$Timeout = 30
)

# Colors
$Red = "`e[31m"
$Green = "`e[32m"
$Yellow = "`e[33m"
$Blue = "`e[34m"
$Cyan = "`e[36m"
$Bold = "`e[1m"
$Reset = "`e[0m"

# Logging functions
function Log-Info { param($msg) Write-Host "${Blue}[INFO]${Reset} $msg" }
function Log-Success { param($msg) Write-Host "${Green}[SUCCESS]${Reset} $msg" }
function Log-Warn { param($msg) Write-Host "${Yellow}[WARN]${Reset} $msg" }
function Log-Error { param($msg) Write-Host "${Red}[ERROR]${Reset} $msg" }
function Log-Step { param($msg) Write-Host "${Cyan}${Bold}[STEP]${Reset} $msg" }

# Get script directory
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectDir = Split-Path -Parent $ScriptDir

# Validate parameters
if ([string]::IsNullOrEmpty($Password) -and [string]::IsNullOrEmpty($KeyFile)) {
    Log-Error "Either Password or KeyFile is required"
    exit 1
}

if (-not [string]::IsNullOrEmpty($KeyFile) -and -not (Test-Path $KeyFile)) {
    Log-Error "SSH key file not found: $KeyFile"
    exit 1
}

# Detect latest version (release artifacts live under build/<version>/ per build-release.ps1)
function Detect-LatestVersion {
    Log-Step "Detecting latest version..."

    if (-not [string]::IsNullOrEmpty($Version)) {
        Log-Info "Using specified version: $Version"
    } else {
        $versionFile = Join-Path $ProjectDir "VERSION"
        if (Test-Path $versionFile) {
            $script:Version = (Get-Content $versionFile).Trim()
        }

        if ([string]::IsNullOrEmpty($Version)) {
            $buildRoot = Join-Path $ProjectDir "build"
            $files = @()
            if (Test-Path $buildRoot) {
                $files = Get-ChildItem -Path $buildRoot -Recurse -Filter "tunnelbypass_*_linux_amd64.tar.gz" -File -ErrorAction SilentlyContinue |
                    Sort-Object LastWriteTime -Descending
            }
            if ($files) {
                $name = $files[0].Name
                if ($name -match 'tunnelbypass_([^_]+)_linux_amd64\.tar\.gz') {
                    $script:Version = $matches[1]
                }
            }
        }

        if ([string]::IsNullOrEmpty($Version)) {
            Log-Error "Could not detect version. Please specify with -Version"
            exit 1
        }

        Log-Success "Detected version: $Version"
    }

    if ([string]::IsNullOrEmpty($ReleaseDir)) {
        $resolved = Join-Path (Join-Path $ProjectDir "build") $Version
        Set-Variable -Name ReleaseDir -Scope Script -Value $resolved
        Log-Info "Release directory (default): $ReleaseDir"
    }
}

# Detect remote system using SSH
function Detect-RemoteSystem {
    Log-Step "Detecting remote system ($Server)..."

    $sshOpts = "-p $Port -o ConnectTimeout=$Timeout -o StrictHostKeyChecking=no"

    if (-not [string]::IsNullOrEmpty($KeyFile)) {
        $sshCmd = "ssh -i `"$KeyFile`" $sshOpts ${User}@${Server}"
    } else {
        # Note: Password auth in PowerShell is more complex, may need plink
        $sshCmd = "ssh $sshOpts ${User}@${Server}"
        $env:SSHPASS = $Password
    }

    try {
        $remoteOs = Invoke-Expression "$sshCmd 'uname -s'" 2>$null
        $remoteArch = Invoke-Expression "$sshCmd 'uname -m'" 2>$null
    }
    catch {
        Log-Error "Failed to connect to remote server ${Server}: $_"
        exit 1
    }

    Log-Info "Remote OS: $remoteOs"
    Log-Info "Remote Architecture: $remoteArch"

    # Map OS
    $mappedOs = switch -Regex ($remoteOs) {
        "Linux" { "linux" }
        "Darwin" {
            Log-Error "Remote macOS is not supported as a TunnelBypass server host; use Linux or Windows."
            exit 1
        }
        "CYGWIN|MINGW|MSYS" { "windows" }
        default {
            Log-Error "Unsupported OS: $remoteOs"
            exit 1
        }
    }

    # Map architecture
    $mappedArch = switch -Regex ($remoteArch) {
        "x86_64|amd64" { "amd64" }
        "aarch64|arm64" { "arm64" }
        "armv7l" { "arm" }
        "i386|i686" { "386" }
        default {
            Log-Error "Unsupported architecture: $remoteArch"
            exit 1
        }
    }

    $script:RemoteOs = $mappedOs
    $script:RemoteArch = $mappedArch

    Log-Success "Mapped to: ${mappedOs}_${mappedArch}"
}

# Select release file
function Select-ReleaseFile {
    Log-Step "Selecting release file..."

    $ext = if ($RemoteOs -eq "windows") { ".exe" } else { ".tar.gz" }
    $script:ReleaseFile = Join-Path $ReleaseDir "tunnelbypass_${Version}_${RemoteOs}_${RemoteArch}${ext}"

    Log-Info "Looking for: $(Split-Path $ReleaseFile -Leaf)"

    if (-not (Test-Path $ReleaseFile)) {
        Log-Error "Release file not found: $ReleaseFile"
        Log-Info "Available files:"
        Get-ChildItem -Path "$ReleaseDir\tunnelbypass_*" -ErrorAction SilentlyContinue | ForEach-Object { "  $($_.Name)" }
        exit 1
    }

    $size = (Get-Item $ReleaseFile).Length / 1KB
    Log-Success "Found: $(Split-Path $ReleaseFile -Leaf) ($([math]::Round($size, 2)) KB)"
}

# Upload file using SCP
function Upload-File {
    Log-Step "Uploading to remote server..."

    if ($DryRun) {
        Log-Warn "[DRY-RUN] Would upload: $ReleaseFile -> /root/tunnelbypass/"
        return
    }

    $destDir = "/root/tunnelbypass"
    $filename = Split-Path $ReleaseFile -Leaf

    Log-Info "Destination: ${User}@${Server}:$destDir/"

    # Create directory
    $sshOpts = "-p $Port -o StrictHostKeyChecking=no"
    if ($KeyFile) {
        $sshCmd = "ssh -i `"$KeyFile`" $sshOpts ${User}@${Server}"
    } else {
        $sshCmd = "ssh $sshOpts ${User}@${Server}"
    }

    Invoke-Expression "$sshCmd 'mkdir -p $destDir'" 2>$null

    # Upload
    if ($KeyFile) {
        $scpCmd = "scp -i `"$KeyFile`" -P $Port -o StrictHostKeyChecking=no `"$ReleaseFile`" ${User}@${Server}:$destDir/"
    } else {
        $scpCmd = "scp -P $Port -o StrictHostKeyChecking=no `"$ReleaseFile`" ${User}@${Server}:$destDir/"
    }

    Log-Info "Uploading $filename..."
    Invoke-Expression $scpCmd

    Log-Success "Upload complete"
}

# Install on remote
function Install-Remote {
    Log-Step "Installing on remote server..."

    if ($DryRun) {
        Log-Warn "[DRY-RUN] Would stop TunnelBypass services, replace binary, verify"
        return
    }

    $sshOpts = "-p $Port -o StrictHostKeyChecking=no"
    if ($KeyFile) {
        $sshCmd = "ssh -i `"$KeyFile`" $sshOpts ${User}@${Server}"
    } else {
        $sshCmd = "ssh $sshOpts ${User}@${Server}"
    }

    $destDir = "/root/tunnelbypass"
    $filename = Split-Path $ReleaseFile -Leaf

    # Stop any systemd units using /usr/local/bin/tunnelbypass so replace does not fail (ETXTBUSY).
    $stopUnits = 'for u in TunnelBypass-SSH-Forwarder TunnelBypass-SSH TunnelBypass-WSS TunnelBypass-SSL TunnelBypass-VLESS-WS TunnelBypass-VLESS TunnelBypass-Hysteria TunnelBypass-WireGuard TunnelBypass-Tunnel TunnelBypass-UDP TunnelBypass-UDPGW; do systemctl stop $u 2>/dev/null || true; done'

    $parts = @()
    if ($RemoteOs -eq "linux") {
        Log-Info "Stopping TunnelBypass services (if any) before replacing binary..."
        $parts += $stopUnits
        $parts += "sleep 1"
    }
    $parts += "cd $destDir"
    $parts += "rm -f tunnelbypass"
    if ($filename -match "\.tar\.gz$") {
        $parts += "tar -xzf $filename"
    }
    $parts += "chmod +x tunnelbypass"
    $parts += "install -m 0755 tunnelbypass /usr/local/bin/tunnelbypass"
    $parts += "tunnelbypass -version"

    $cmds = $parts -join " && "

    Log-Info "Running installation commands..."
    Invoke-Expression "$sshCmd '$cmds'"

    Log-Success "Installation complete"
}

# Restart services
function Restart-Services {
    if ($SkipRestart) {
        Log-Warn "Skipping service restart"
        return
    }

    if ($RemoteOs -eq "windows") {
        Log-Warn "Windows detected, skipping systemd service restart"
        return
    }

    Log-Step "Restarting services..."

    if ($DryRun) {
        Log-Warn "[DRY-RUN] Would restart services"
        return
    }

    $sshOpts = "-p $Port -o StrictHostKeyChecking=no"
    if ($KeyFile) {
        $sshCmd = "ssh -i `"$KeyFile`" $sshOpts ${User}@${Server}"
    } else {
        $sshCmd = "ssh $sshOpts ${User}@${Server}"
    }

    $cmds = @(
        "systemctl daemon-reload"
        "systemctl restart TunnelBypass-UDPGW 2>/dev/null || true"
        "systemctl restart TunnelBypass-SSH 2>/dev/null || true"
        "systemctl restart TunnelBypass-SSH-Forwarder 2>/dev/null || true"
        "systemctl restart TunnelBypass-SSL 2>/dev/null || true"
        "systemctl restart TunnelBypass-WSS 2>/dev/null || true"
        "systemctl restart TunnelBypass-VLESS-WS 2>/dev/null || true"
        "systemctl restart TunnelBypass-VLESS 2>/dev/null || true"
        "systemctl restart TunnelBypass-Hysteria 2>/dev/null || true"
        "systemctl restart TunnelBypass-WireGuard 2>/dev/null || true"
        "systemctl restart TunnelBypass-Tunnel 2>/dev/null || true"
        "systemctl restart TunnelBypass-UDP 2>/dev/null || true"
    ) -join " && "

    Log-Info "Restarting services..."
    Invoke-Expression "$sshCmd '$cmds'" -ErrorAction SilentlyContinue

    Log-Success "Services restarted"
}

# Validate installation
function Validate-Installation {
    Log-Step "Validating installation..."

    if ($DryRun) {
        Log-Warn "[DRY-RUN] Would validate"
        return
    }

    $sshOpts = "-p $Port -o StrictHostKeyChecking=no"
    if ($KeyFile) {
        $sshCmd = "ssh -i `"$KeyFile`" $sshOpts ${User}@${Server}"
    } else {
        $sshCmd = "ssh $sshOpts ${User}@${Server}"
    }

    Log-Info "Checking version..."
    $version = Invoke-Expression "$sshCmd 'tunnelbypass -version'" 2>$null

    if ($version -match "VERSION_CHECK_FAILED|not found") {
        Log-Error "Validation failed"
        exit 1
    }

    Log-Success "Validation successful: $version"
}

# Main
Write-Host "${Bold}"
Write-Host "=============================================================="
Write-Host "          TunnelBypass Deployment Script (Windows)"
Write-Host "=============================================================="
Write-Host "${Reset}"

if ($DryRun) {
    Log-Warn "DRY-RUN MODE: No changes will be made"
    Write-Host ""
}

Detect-LatestVersion
Detect-RemoteSystem
Select-ReleaseFile
Upload-File
Install-Remote
Restart-Services
Validate-Installation

Write-Host ""
Write-Host "=============================================================="
Log-Success "Deployment Complete!"
Write-Host "=============================================================="
Write-Host ""
Write-Host "Server: ${Server}:$Port"
Write-Host "User: $User"
Write-Host "OS/Arch: ${RemoteOs}_${RemoteArch}"
Write-Host "Version: $Version"
Write-Host "Binary: /usr/local/bin/tunnelbypass"
Write-Host ""
Write-Host "Next steps:"
Write-Host "  1. SSH into server: ssh $User@$Server"
Write-Host "  2. Run wizard: sudo tunnelbypass wizard"
Write-Host ""
