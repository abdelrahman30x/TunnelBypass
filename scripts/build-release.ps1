[CmdletBinding()]
param (
    [string]$Version = ""
)

# Build and package release assets into ./build/<version>/ (PowerShell).
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

$outDir = Join-Path (Join-Path (Get-Location) "build") $Version
New-Item -ItemType Directory -Force -Path $outDir | Out-Null

Write-Host "Building release for version: $Version"
Write-Host "Output directory: $outDir"

$Targets = @(
    @{ OS = "linux"; Arch = "amd64"; Ext = "" },
    @{ OS = "linux"; Arch = "arm64"; Ext = "" },
    @{ OS = "windows"; Arch = "amd64"; Ext = ".exe" },
    @{ OS = "windows"; Arch = "arm64"; Ext = ".exe" }
)

foreach ($t in $Targets) {
    $os = $t.OS
    $arch = $t.Arch
    $ext = $t.Ext

    $binName = "tunnelbypass$ext"
    $binPath = Join-Path $outDir $binName

    if ($ext -eq ".exe") {
        $assetName = "tunnelbypass_${Version}_${os}_${arch}.exe"
    } else {
        $assetName = "tunnelbypass_${Version}_${os}_${arch}.tar.gz"
    }
    $assetPath = Join-Path $outDir $assetName

    Write-Host "-> Building $os/$arch as $assetName"

    $env:GOOS = $os
    $env:GOARCH = $arch

    go build -trimpath -ldflags "-s -w -X main.Version=$Version" -o $binPath ./cmd
    if ($LASTEXITCODE -ne 0 -and $LASTEXITCODE -ne $null) { throw "Build failed for $os/$arch" }

    if ($os -ne "windows") {
        if (Test-Path $binPath) {
            Push-Location $outDir
            try {
                tar -czf $assetName $binName
            } finally {
                Pop-Location
            }
            Remove-Item $binPath -Force
        } else {
            Write-Warning "Binary $binPath not found, skipping packaging for $os/$arch"
        }
    } else {
        if (Test-Path $assetPath) { Remove-Item $assetPath -Force }
        if (Test-Path $binPath) {
            Rename-Item -Path $binPath -NewName $assetName -Force
        } else {
            Write-Warning "Binary $binPath not found, skipping packaging for $os/$arch"
        }
    }
}

Write-Host "=================="
Write-Host "All release assets created successfully:"
Get-ChildItem -Path $outDir -Filter "tunnelbypass_${Version}_*" | Select-Object Name, Length | Format-Table
