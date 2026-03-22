[CmdletBinding()]
param (
    [string]$Version = ""
)

# Build and package all standard release assets (PowerShell).
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# Go to repo root
Set-Location (Split-Path -Parent $PSScriptRoot)

if ([string]::IsNullOrWhiteSpace($Version)) {
    if (Test-Path "VERSION") {
        $Version = (Get-Content "VERSION" -Raw).Trim()
    } else {
        $Version = try { git describe --tags --abbrev=0 2>$null } catch { "v0.0.0" }
        if (-not $Version) { $Version = "v0.0.0" }
    }
}

Write-Host "Building release for version: $Version"

$Targets = @(
    @{ OS = "linux"; Arch = "amd64"; Ext = "" },
    @{ OS = "linux"; Arch = "arm64"; Ext = "" },
    @{ OS = "windows"; Arch = "amd64"; Ext = ".exe" },
    @{ OS = "windows"; Arch = "arm64"; Ext = ".exe" },
    @{ OS = "darwin"; Arch = "amd64"; Ext = "" },
    @{ OS = "darwin"; Arch = "arm64"; Ext = "" }
)

foreach ($t in $Targets) {
    $os = $t.OS
    $arch = $t.Arch
    $ext = $t.Ext
    
    $binName = "tunnelbypass$ext"
    
    if ($ext -eq ".exe") {
        $assetName = "tunnelbypass_${Version}_${os}_${arch}.exe"
    } else {
        $assetName = "tunnelbypass_${Version}_${os}_${arch}.tar.gz"
    }
    
    Write-Host "-> Building $os/$arch as $assetName"
    
    $env:GOOS = $os
    $env:GOARCH = $arch
    
    # Build
    go build -trimpath -ldflags "-s -w -X main.Version=$Version" -o $binName ./cmd
    if ($LASTEXITCODE -ne 0 -and $LASTEXITCODE -ne $null) { throw "Build failed for $os/$arch" }
    
    # Package
    if ($os -ne "windows") {
        # Use native Windows 10+ tar for .tar.gz creation
        if (Test-Path $binName) {
            tar -czf $assetName $binName
            Remove-Item $binName -Force
        } else {
            Write-Warning "Binary $binName not found, skipping packaging for $os/$arch"
        }
    } else {
        if (Test-Path $assetName) { Remove-Item $assetName -Force }
        if (Test-Path $binName) {
            Rename-Item -Path $binName -NewName $assetName -Force
        } else {
            Write-Warning "Binary $binName not found, skipping packaging for $os/$arch"
        }
    }
}

Write-Host "=================="
Write-Host "All release assets created successfully:"
Get-ChildItem -Filter "tunnelbypass_${Version}_*.*" | Select-Object Name, Length | Format-Table
