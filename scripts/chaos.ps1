# Chaos scenario: stop the database, check that the API degrades safely, start it again
# and measure how long the API needs to recover. ASCII only on purpose.
param(
    [string]$BaseUrl = "http://localhost:8080",
    [string]$Password = "Demo!2026Parola",
    [int]$RecoveryTimeoutSeconds = 90
)

$ErrorActionPreference = "Stop"
Set-Location (Split-Path -Parent $PSScriptRoot)
$script:failures = 0

function Check {
    param([string]$Name, [bool]$Condition)
    if ($Condition) { Write-Host "PASS  $Name" -ForegroundColor Green }
    else { Write-Host "FAIL  $Name" -ForegroundColor Red; $script:failures++ }
}

function Get-Status {
    param([string]$Method, [string]$Path, [string]$Body)
    $params = @{ Method = $Method; Uri = "$BaseUrl$Path"; UseBasicParsing = $true; TimeoutSec = 10 }
    if ($Body) { $params["Body"] = $Body; $params["ContentType"] = "application/json" }
    try { return [int](Invoke-WebRequest @params).StatusCode }
    catch {
        if ($_.Exception.Response) { return [int]$_.Exception.Response.StatusCode }
        return 0
    }
}

$loginBody = '{"email":"operator@hatkontrol.test","password":"' + $Password + '"}'

Write-Host "== Chaos: database outage ==" -ForegroundColor Cyan
Check "before: health is 200" ((Get-Status Get "/health") -eq 200)
Check "before: login is 200" ((Get-Status Post "/auth/login" $loginBody) -eq 200)

Write-Host "Stopping the database..."
docker compose stop db | Out-Null
Start-Sleep -Seconds 2

$healthDown = Get-Status Get "/health"
Check "during outage: health reports 503 (not a hang, not 200)" ($healthDown -eq 503)
$loginDown = Get-Status Post "/auth/login" $loginBody
Check "during outage: login fails with 500 and does not crash the API" ($loginDown -eq 500)
$backendState = docker compose ps backend --format "{{.State}}"
Check "during outage: backend container keeps running" ($backendState -match "running")

Write-Host "Starting the database again..."
docker compose start db | Out-Null
$watch = [System.Diagnostics.Stopwatch]::StartNew()
$recovered = $false
while ($watch.Elapsed.TotalSeconds -lt $RecoveryTimeoutSeconds) {
    if ((Get-Status Get "/health") -eq 200) { $recovered = $true; break }
    Start-Sleep -Seconds 1
}
$watch.Stop()
$seconds = [math]::Round($watch.Elapsed.TotalSeconds, 1)

Check "after: health is 200 again within $RecoveryTimeoutSeconds s (took $seconds s), no restart of backend" $recovered
Check "after: login works again" ((Get-Status Post "/auth/login" $loginBody) -eq 200)

Write-Host ""
if ($script:failures -eq 0) { Write-Host "ALL CHECKS PASSED (recovery $seconds s)" -ForegroundColor Green; exit 0 }
Write-Host "$($script:failures) CHECK(S) FAILED" -ForegroundColor Red
exit 1