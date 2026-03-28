#Requires -Version 5.1
<#
.SYNOPSIS
  TunnelBypass one-line installer for Windows (PowerShell).

.EXAMPLE
  irm https://raw.githubusercontent.com/abdelrahman30x/TunnelBypass/main/scripts/install.ps1 | iex

  If execution policy blocks iex, run once:
    Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass

  Default: latest published GitHub release (no version variable needed).
  Optional environment variables:
    $env:INSTALL_OWNER   (default: abdelrahman30x)
    $env:INSTALL_REPO    (default: TunnelBypass)
    $env:INSTALL_VERSION only to pin a tag (e.g. v1.2.1); leave unset for latest
    $env:INSTALL_PREFIX  (directory to install tunnelbypass.exe; default: current directory)
#>

[CmdletBinding()]
param(
    [string]$InstallDir = "",
    [string]$Owner = "",
    [string]$Repo = "",
    [string]$Version = ""
)

try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
} catch {}
$ErrorActionPreference = "Stop"

if (-not $Owner) { $Owner = if ($env:INSTALL_OWNER) { $env:INSTALL_OWNER } else { "abdelrahman30x" } }
if (-not $Repo)  { $Repo  = if ($env:INSTALL_REPO)  { $env:INSTALL_REPO }  else { "TunnelBypass" } }
if (-not $Version) { $Version = $env:INSTALL_VERSION }

$arch = $env:PROCESSOR_ARCHITECTURE
if ($arch -eq "AMD64") { $archGo = "amd64" }
elseif ($arch -eq "ARM64") { $archGo = "arm64" }
else {
    Write-Error "Unsupported architecture: $arch (need AMD64 or ARM64)"
}

$wantSub = "_windows_${archGo}"
if (-not $InstallDir) {
    $InstallDir = if ($env:INSTALL_PREFIX) { $env:INSTALL_PREFIX } else { (Get-Location).Path }
}

$api = if ($Version) {
    "https://api.github.com/repos/$Owner/$Repo/releases/tags/$Version"
} else {
    "https://api.github.com/repos/$Owner/$Repo/releases/latest"
}

Write-Host "[*] Fetching release metadata..."
$headers = @{ "User-Agent" = "TunnelBypass-Install"; "Accept" = "application/vnd.github+json" }
$rel = Invoke-RestMethod -Uri $api -Headers $headers -UseBasicParsing

if ($rel.tag_name) {
    if ($Version) {
        Write-Host "[*] Release tag: $($rel.tag_name) (pinned)"
    } else {
        Write-Host "[*] Latest release: $($rel.tag_name)"
    }
}

$asset = $rel.assets | Where-Object { $_.name -like "*${wantSub}*" -and $_.name -like "*.exe" } | Select-Object -First 1
if (-not $asset) {
    $asset = $rel.assets | Where-Object { $_.name -match 'windows.*\.exe$' } | Select-Object -First 1
}
if (-not $asset) {
    throw "No Windows .exe asset found for $wantSub. See https://github.com/$Owner/$Repo/releases"
}

$url = $asset.browser_download_url
$name = $asset.name
Write-Host "[*] Downloading: $name"

$tmp = Join-Path $env:TEMP ("tb-setup-" + [Guid]::NewGuid().ToString("n") + ".exe")
try {
    Invoke-WebRequest -Uri $url -OutFile $tmp -Headers @{ "User-Agent" = "TunnelBypass-Install" } -UseBasicParsing
} catch {
    throw "Download failed: $_"
}

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$dest = Join-Path $InstallDir "tunnelbypass.exe"
Copy-Item -Path $tmp -Destination $dest -Force
Remove-Item -Force $tmp

Write-Host "[+] Installed: $dest"
Write-Host "    Run: .\tunnelbypass.exe --version"
$inPath = $false
foreach ($p in ($env:Path -split ';')) {
    if ($p -and (Test-Path $p) -and ((Resolve-Path $InstallDir).Path -eq (Resolve-Path $p).Path)) { $inPath = $true; break }
}
if (-not $inPath) {
    Write-Host "[!] If needed, add to PATH: $InstallDir"
}
