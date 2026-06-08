$ErrorActionPreference = "Continue"
$OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$exe = Join-Path $root "tmp\auto-checkin-bot.exe"
$supervisorLog = Join-Path $root "supervisor.log"

function Write-SupervisorLog {
    param([string]$Message)
    $line = "[{0}] {1}" -f (Get-Date -Format "yyyy-MM-dd HH:mm:ss"), $Message
    Add-Content -LiteralPath $supervisorLog -Value $line -Encoding UTF8
}

while ($true) {
    if (-not (Test-Path -LiteralPath $exe)) {
        Write-SupervisorLog "Missing executable: $exe"
        Start-Sleep -Seconds 5
        continue
    }

    Write-SupervisorLog "Starting auto-checkin-bot.exe"
    try {
        & $exe
        $exitCode = $LASTEXITCODE
        Write-SupervisorLog "auto-checkin-bot.exe exited with code: $exitCode"
    } catch {
        Write-SupervisorLog "auto-checkin-bot.exe failed: $($_.Exception.Message)"
    }

    Start-Sleep -Seconds 5
}
