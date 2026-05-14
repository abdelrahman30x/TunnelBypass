#Requires -Version 5.1
<#
.SYNOPSIS
  TunnelBypass one-line installer for Windows (PowerShell).

.EXAMPLE
  irm https://raw.githubusercontent.com/abdelrahman30x/TunnelBypass/main/scripts/install.ps1 | iex

  If execution policy blocks iex, run once:
    Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass

  Default: latest published GitHub release (no version variable needed).
  Installs to %LOCALAPPDATA%\TunnelBypass, adds that folder to your user PATH, and writes TunnelBypass.cmd / tb.cmd shims.
  Re-running is safe: the install directory is only added to user PATH once (not duplicated).

  Optional environment variables:
    $env:INSTALL_OWNER   (default: abdelrahman30x)
    $env:INSTALL_REPO    (default: TunnelBypass)
    $env:INSTALL_VERSION only to pin a tag (e.g. v1.2.1); leave unset for latest
    $env:INSTALL_PREFIX  override install directory (default: %LOCALAPPDATA%\TunnelBypass)
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
    if ($env:INSTALL_PREFIX) {
        $InstallDir = $env:INSTALL_PREFIX
    } else {
        $la = $env:LOCALAPPDATA
        if (-not $la) { $la = Join-Path $env:USERPROFILE "AppData\Local" }
        $InstallDir = Join-Path $la "TunnelBypass"
    }
}

$InstallDir = [System.IO.Path]::GetFullPath($InstallDir)

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

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$dest = Join-Path $InstallDir "tunnelbypass.exe"
Write-Host "[*] Downloading: $name"
$tmp = Join-Path $env:TEMP ("tb-setup-" + [Guid]::NewGuid().ToString("n") + ".exe")
try {
    Invoke-WebRequest -Uri $url -OutFile $tmp -Headers @{ "User-Agent" = "TunnelBypass-Install" } -UseBasicParsing
} catch {
    throw "Download failed: $_"
}
Copy-Item -Path $tmp -Destination $dest -Force
Remove-Item -Force $tmp

# CMD shims so "TunnelBypass" / "tb" resolve (PATHEXT includes .cmd)
$shim = "@echo off`r`n" + '"%~dp0tunnelbypass.exe" %*' + "`r`n"
Set-Content -LiteralPath (Join-Path $InstallDir "TunnelBypass.cmd") -Value $shim -Encoding ascii -NoNewline
Set-Content -LiteralPath (Join-Path $InstallDir "tb.cmd") -Value $shim -Encoding ascii -NoNewline

function Add-TunnelBypassUserPath {
    param([string]$Dir)
    $d = $Dir.TrimEnd('\')
    $cur = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($null -eq $cur) { $cur = '' }
    $found = $false
    foreach ($x in ($cur -split ';')) {
        $t = $x.Trim().TrimEnd('\')
        if ($t -ne '' -and $t -ieq $d) { $found = $true; break }
    }
    if (-not $found) {
        $n = if ($cur.Trim() -eq '') { $d } else { ($cur.TrimEnd(';') + ';' + $d) }
        [Environment]::SetEnvironmentVariable('Path', $n, 'User')
        Write-Host "[+] Added to user PATH: $d"
    } else {
        Write-Host "[*] User PATH already lists: $d"
    }
    try {
        if (-not ('TunnelBypassEnvBroadcast' -as [type])) {
            Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class TunnelBypassEnvBroadcast {
  [DllImport("user32.dll", CharSet=CharSet.Auto, SetLastError=false)]
  public static extern IntPtr SendMessageTimeout(IntPtr hWnd, uint Msg, IntPtr wParam, string lParam, uint fuFlags, uint uTimeout, out IntPtr lpdwResult);
}
'@
        }
        [IntPtr]$r = [IntPtr]::Zero
        [void][TunnelBypassEnvBroadcast]::SendMessageTimeout([IntPtr]0xffff, 26, [IntPtr]::Zero, "Environment", 2, 5000, [ref]$r)
    } catch {}
}

Add-TunnelBypassUserPath -Dir $InstallDir

Write-Host "[+] Installed: $dest"
try {
    $verOut = & $dest --version 2>$null
    if ($verOut) { Write-Host "    Version: $verOut" }
} catch {}

$sessionHas = $false
foreach ($p in ($env:Path -split ';')) {
    $t = $p.Trim().TrimEnd('\')
    if ($t -ne '' -and $t -ieq $InstallDir.TrimEnd('\')) { $sessionHas = $true; break }
}
if (-not $sessionHas) {
    Write-Host "[i] This window was started before PATH changed. Use a new terminal, or for this session:"
    Write-Host "    `$env:Path = `"$InstallDir;`$env:Path`""
    Write-Host "    (cmd.exe)  set PATH=$InstallDir;%PATH%"
}

Write-Host "    From any folder (after PATH is visible): tunnelbypass  |  TunnelBypass  |  tb"
