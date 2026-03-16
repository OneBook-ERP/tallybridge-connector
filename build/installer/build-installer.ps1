# build-installer.ps1 — Build TallyBridge Inno Setup installer(s)
# Run from repo root or the installer directory.
#
# Usage:
#   .\build\installer\build-installer.ps1                # x64 only, version from git tag
#   .\build\installer\build-installer.ps1 -Arch x64      # explicit arch
#   .\build\installer\build-installer.ps1 -Arch all      # x64 + x86 + arm64
#   .\build\installer\build-installer.ps1 -Version 1.2.3 -Arch x64

param(
    [string]$Arch    = "x64",
    [string]$Version = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# Resolve repo root
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot   = Resolve-Path (Join-Path $scriptDir "..\..")
$issFile    = Join-Path $scriptDir "tallybridge-setup.iss"

# Auto-detect version from git tag if not provided
if ($Version -eq "") {
    try {
        $Version = (git -C $repoRoot describe --tags --abbrev=0 2>$null).TrimStart("v")
    } catch {}
    if ($Version -eq "") { $Version = "1.0.0" }
}

Write-Host "=== TallyBridge Installer Build ===" -ForegroundColor Cyan
Write-Host "  Version : $Version"
Write-Host "  Arch    : $Arch"
Write-Host "  ISS     : $issFile"

# Create output dir
$distDir = Join-Path $repoRoot "dist"
New-Item -ItemType Directory -Force -Path $distDir | Out-Null

function Build-Installer([string]$arch) {
    Write-Host ""
    Write-Host "--- Building installer for $arch ---" -ForegroundColor Yellow

    $exeName = switch ($arch) {
        "arm64" { "tallybridge-connector-arm64.exe" }
        "x86"   { "tallybridge-connector-x86.exe" }
        default { "tallybridge-connector-x64.exe" }
    }

    $exePath = Join-Path $repoRoot "build\bin\$exeName"
    if (-not (Test-Path $exePath)) {
        Write-Error "Executable not found: $exePath — run build-windows.sh first"
        exit 1
    }

    & iscc `
        /DAppVersion=$Version `
        /DArch=$arch `
        $issFile

    if ($LASTEXITCODE -ne 0) {
        Write-Error "iscc failed (exit $LASTEXITCODE)"
        exit $LASTEXITCODE
    }

    Write-Host "  Done." -ForegroundColor Green
}

switch ($Arch) {
    "all" {
        Build-Installer "x64"
        Build-Installer "x86"
        Build-Installer "arm64"
    }
    default {
        Build-Installer $Arch
    }
}

Write-Host ""
Write-Host "=== Installers written to: $distDir ===" -ForegroundColor Cyan
Get-ChildItem (Join-Path $distDir "tallybridge-connector-setup-*.exe") |
    Select-Object Name, @{N="Size";E={"{0:N2} MB" -f ($_.Length/1MB)}} |
    Format-Table -AutoSize
