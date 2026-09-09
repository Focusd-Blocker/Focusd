# focusd Windows installer entry point.
# This wrapper can be run from the repository root or another working directory.

$ErrorActionPreference = "Stop"

$ProjectRoot = Split-Path -Parent $MyInvocation.MyCommand.Definition
$DaemonInstaller = Join-Path $ProjectRoot "daemon\install.ps1"

if (-not (Test-Path -LiteralPath $DaemonInstaller -PathType Leaf)) {
    throw "Could not find daemon\install.ps1 next to this installer."
}

$principal = New-Object Security.Principal.WindowsPrincipal(
    [Security.Principal.WindowsIdentity]::GetCurrent()
)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    $process = Start-Process powershell.exe `
        -Verb RunAs `
        -Wait `
        -PassThru `
        -ArgumentList @(
            "-NoProfile",
            "-ExecutionPolicy", "Bypass",
            "-File", "`"$($MyInvocation.MyCommand.Definition)`""
        )
    exit $process.ExitCode
}

& $DaemonInstaller
if ($LASTEXITCODE -and $LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}
