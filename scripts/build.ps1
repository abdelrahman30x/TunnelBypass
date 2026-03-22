# Build tunnelbypass.exe from repo root (PowerShell).
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
Set-Location (Split-Path -Parent $PSScriptRoot)
go build -o tunnelbypass.exe ./cmd
Write-Host "OK: .\tunnelbypass.exe"
