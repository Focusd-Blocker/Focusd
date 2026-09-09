# focusd Windows installer.
# Can be run from any working directory.

$ErrorActionPreference = "Stop"

$ScriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Definition

# Relaunch once with the normal Windows UAC prompt so users do not need to
# change execution policy or manually open an elevated PowerShell window.
$principal = New-Object Security.Principal.WindowsPrincipal(
    [Security.Principal.WindowsIdentity]::GetCurrent()
)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    $scriptPath = $MyInvocation.MyCommand.Definition
    $process = Start-Process powershell.exe `
        -Verb RunAs `
        -Wait `
        -PassThru `
        -ArgumentList @(
            "-NoProfile",
            "-ExecutionPolicy", "Bypass",
            "-File", "`"$scriptPath`""
        )
    exit $process.ExitCode
}

$BIN_NAME = "focusd.exe"
$INSTALL_DIR = Join-Path $env:LOCALAPPDATA "focusd\bin"
$CACHE_DIR = Join-Path $env:LOCALAPPDATA "focusd"
$EXTENSION_DIR = Join-Path $CACHE_DIR "extension"
$STARTUP_FOLDER = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\Startup"
$XPI_PATH = Join-Path $EXTENSION_DIR "focusd.xpi"
$POLICY_ID = "{5f1c1d4d-c0f0-41bc-862b-0c7f8b860beb}"

function Require-Command($Name, $InstallHint) {
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "$Name was not found. $InstallHint"
    }
}

Require-Command "go" "Install Go from https://go.dev/dl/ and run this installer again."
Require-Command "curl.exe" "Install Windows curl or use a current Windows 10/11 installation."

Push-Location $ScriptRoot
try {
    Write-Host "==> Building $BIN_NAME"
    $env:CGO_ENABLED = "0"
    go build -trimpath -ldflags="-s -w -H windowsgui" -o $BIN_NAME .
    if ($LASTEXITCODE -ne 0) {
        throw "Go build failed."
    }

    Write-Host "==> Installing daemon to $INSTALL_DIR"
    New-Item -ItemType Directory -Force -Path $INSTALL_DIR | Out-Null
    Copy-Item -LiteralPath (Join-Path $ScriptRoot $BIN_NAME) `
        -Destination (Join-Path $INSTALL_DIR $BIN_NAME) -Force

    Write-Host "==> Downloading browser extension"
    New-Item -ItemType Directory -Force -Path $EXTENSION_DIR | Out-Null
    $download = "https://github.com/Focusd-Blocker/Focusd/releases/latest/download/focusd.xpi"
    $tempXpi = "$XPI_PATH.tmp"
    curl.exe -fL --retry 3 --retry-delay 2 -o $tempXpi $download
    if ($LASTEXITCODE -ne 0) {
        Remove-Item -LiteralPath $tempXpi -Force -ErrorAction SilentlyContinue
        throw "Extension download failed."
    }
    Move-Item -LiteralPath $tempXpi -Destination $XPI_PATH -Force

    $XPI_URL = "file:///" + ([uri]::EscapeUriString(($XPI_PATH -replace '\\', '/')))
    $POLICY_JSON = @"
{
  "policies": {
    "ExtensionSettings": {
      "$POLICY_ID": {
        "installation_mode": "force_installed",
        "install_url": "$XPI_URL",
        "private_browsing": true
      }
    }
  }
}
"@

    Write-Host "==> Deploying Firefox-family policies"
    $policyTargets = @(
        "$env:ProgramFiles\Mozilla Firefox\distribution",
        "$env:ProgramFiles(x86)\Mozilla Firefox\distribution",
        "$env:LOCALAPPDATA\Mozilla Firefox\distribution",
        "$env:ProgramFiles\LibreWolf\distribution",
        "$env:LOCALAPPDATA\LibreWolf\distribution",
        "$env:ProgramFiles\Waterfox\distribution",
        "$env:LOCALAPPDATA\Waterfox\distribution",
        "$env:ProgramFiles\Floorp\distribution",
        "$env:LOCALAPPDATA\Floorp\distribution"
    ) | Where-Object { $_ -and (Test-Path (Split-Path $_ -Parent)) } | Select-Object -Unique

    $policyCount = 0
    foreach ($target in $policyTargets) {
        try {
            New-Item -ItemType Directory -Force -Path $target | Out-Null
            $policyPath = Join-Path $target "policies.json"
            $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
            [System.IO.File]::WriteAllText($policyPath, $POLICY_JSON, $utf8NoBom)
            Write-Host "  wrote $target\policies.json"
            $policyCount++
        } catch {
            Write-Warning "Could not write $target\policies.json: $($_.Exception.Message)"
        }
    }

    if ($policyCount -eq 0) {
        Write-Warning "No Firefox-family installation was found or writable. Install Firefox, LibreWolf, Waterfox, or Floorp, then rerun this installer."
    }

    Write-Host "==> Configuring automatic startup"
    New-Item -ItemType Directory -Force -Path $STARTUP_FOLDER | Out-Null
    $shortcutPath = Join-Path $STARTUP_FOLDER "focusd.lnk"
    $wshShell = New-Object -ComObject WScript.Shell
    $shortcut = $wshShell.CreateShortcut($shortcutPath)
    $shortcut.TargetPath = Join-Path $INSTALL_DIR $BIN_NAME
    $shortcut.WorkingDirectory = $INSTALL_DIR
    $shortcut.Description = "focusd - focus daemon"
    $shortcut.Save()

    Write-Host "==> Starting daemon"
    Get-Process focusd -ErrorAction SilentlyContinue |
        Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Process -FilePath (Join-Path $INSTALL_DIR $BIN_NAME) -WorkingDirectory $INSTALL_DIR

    Start-Sleep -Seconds 1
    if (-not (Get-Process focusd -ErrorAction SilentlyContinue)) {
        throw "focusd did not start. Check Windows Event Viewer or run $INSTALL_DIR\$BIN_NAME manually."
    }

    Write-Host ""
    Write-Host "Focusd installed successfully."
    Write-Host "Dashboard: http://127.0.0.1:36287/"
} finally {
    Pop-Location
}
