# Quick verification (section 10 of the case brief).
# Runs against a running, seeded system. ASCII only on purpose.
param(
    [string]$BaseUrl = "http://localhost:8080",
    [string]$Password = "Demo!2026Parola"
)

$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$script:results = @()

function Add-Result {
    param([string]$Id, [string]$Status, [string]$Note)
    $script:results += [pscustomobject]@{ Id = $Id; Status = $Status; Note = $Note }
}

function Invoke-Api {
    param([string]$Method, [string]$Path, [string]$Token, [string]$Body)
    $headers = @{}
    if ($Token) { $headers["Authorization"] = "Bearer $Token" }
    $params = @{
        Method          = $Method
        Uri             = "$BaseUrl$Path"
        Headers         = $headers
        UseBasicParsing = $true
    }
    if ($Body) {
        $params["Body"] = $Body
        $params["ContentType"] = "application/json"
    }

    $status = 0
    $text = ""
    try {
        $response = Invoke-WebRequest @params
        $status = [int]$response.StatusCode
        $text = $response.Content
    } catch {
        if ($_.Exception.Response) { $status = [int]$_.Exception.Response.StatusCode }
    }

    $parsed = $null
    if ($text) {
        try { $parsed = $text | ConvertFrom-Json } catch { $parsed = $null }
    }
    return @{ Status = $status; Body = $parsed }
}

function Invoke-Login {
    param([string]$Email, [string]$LoginPassword)
    $body = '{"email":"' + $Email + '","password":"' + $LoginPassword + '"}'
    return Invoke-Api -Method Post -Path "/auth/login" -Body $body
}

# Runs one of the end-to-end scripts in a child process and returns its exit code and output.
function Invoke-ChildScript {
    param([string]$Name)
    $path = Join-Path $scriptDir $Name
    $previous = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -File $path -BaseUrl $BaseUrl -Password $Password 2>&1 | ForEach-Object { "$_" }
    $code = $LASTEXITCODE
    $ErrorActionPreference = $previous
    return @{ Code = $code; Output = @($output) }
}

function Show-Details {
    param($Run, [string]$Title)
    $lines = @($Run.Output | Where-Object { $_ -match "^FAIL" -or $_ -match "^\s+race:" })
    if ($lines.Count -gt 0) {
        Write-Host "  [$Title]" -ForegroundColor Yellow
        foreach ($line in $lines) { Write-Host "    $line" }
    }
}

Write-Host "== HatKontrol quick verification: $BaseUrl ==" -ForegroundColor Cyan
Write-Host "Running checks, this takes about a minute..."

# --- Health ---
$health = Invoke-Api -Method Get -Path "/health"
$healthOk = $health.Status -eq 200 -and $health.Body -and $health.Body.status -eq "ok" -and $health.Body.database -eq "ok"
Add-Result "HEALTH" $(if ($healthOk) { "PASS" } else { "FAIL" }) "GET /health returns 200 with database ok"

# --- Authentication and authorization ---
$wrong = Invoke-Login "operator@hatkontrol.test" "definitely-wrong-password"
$unknown = Invoke-Login "nobody@hatkontrol.test" "definitely-wrong-password"
$authOk = $wrong.Status -eq 401 -and $unknown.Status -eq 401
Add-Result "AUTH-1" $(if ($authOk) { "PASS" } else { "FAIL" }) "wrong password and unknown e-mail both return 401"

$good = Invoke-Login "operator@hatkontrol.test" $Password
$noToken = Invoke-Api -Method Get -Path "/me"
$garbage = Invoke-Api -Method Get -Path "/me" -Token "not.a.token"
$tokenOk = $good.Status -eq 200 -and $noToken.Status -eq 401 -and $garbage.Status -eq 401
Add-Result "AUTH-2" $(if ($tokenOk) { "PASS" } else { "FAIL" }) "valid login 200, missing or invalid token 401"

$operatorToken = ""
if ($good.Status -eq 200) { $operatorToken = $good.Body.access_token }
$forbidden = Invoke-Api -Method Post -Path "/charts" -Token $operatorToken -Body '{"name":"verify-role-check","type":"individual"}'
Add-Result "AUTH-3" $(if ($forbidden.Status -eq 403) { "PASS" } else { "FAIL" }) "operator creating a chart is rejected with 403"

# --- End-to-end scripts ---
$spc = Invoke-ChildScript "demo-spc.ps1"
$race = Invoke-ChildScript "demo-stock-race.ps1"
$spcPassed = $spc.Code -eq 0
$racePassed = $race.Code -eq 0

Add-Result "T-1" $(if ($spcPassed) { "PASS" } else { "FAIL" }) "SPC rule engine, scripts/demo-spc.ps1 (12 checks)"
Add-Result "T-4" $(if ($racePassed) { "PASS" } else { "FAIL" }) "work order state machine and stock race, scripts/demo-stock-race.ps1 (26 checks)"

Add-Result "F-1" $(if ($racePassed) { "PASS" } else { "FAIL" }) "work orders and BOM through the API (same script as T-4)"
Add-Result "F-2" $(if ($racePassed) { "PARTIAL" } else { "FAIL" }) "counters, scrap and rework work; downtime reasons are not implemented"
Add-Result "F-3" $(if ($spcPassed) { "PARTIAL" } else { "FAIL" }) "measurement entry works through SPC charts; no separate quality form definition"
Add-Result "F-4" $(if ($spcPassed) { "PASS" } else { "FAIL" }) "control charts and alarms through API and web UI (same script as T-1)"
Add-Result "F-5" "NOT-DONE" "NCR management is not implemented"
Add-Result "F-6" "NOT-DONE" "OEE dashboard is not implemented"
Add-Result "F-7" "NOT-DONE" "batch and serial traceability reports are not implemented"
Add-Result "T-2" "NOT-DONE" "OEE with late events is not implemented"
Add-Result "T-3" "NOT-DONE" "forward and backward genealogy is not implemented"
Add-Result "T-5" "NOT-DONE" "high-volume sensor ingestion is not implemented"

# --- Report ---
Write-Host ""
Write-Host ("{0,-8} {1,-9} {2}" -f "ID", "RESULT", "NOTE")
Write-Host ("{0,-8} {1,-9} {2}" -f "--", "------", "----")
foreach ($row in $script:results) {
    $color = switch ($row.Status) {
        "PASS"     { "Green" }
        "FAIL"     { "Red" }
        "PARTIAL"  { "Yellow" }
        default    { "DarkGray" }
    }
    Write-Host ("{0,-8} {1,-9} {2}" -f $row.Id, $row.Status, $row.Note) -ForegroundColor $color
}

$pass = @($script:results | Where-Object { $_.Status -eq "PASS" }).Count
$fail = @($script:results | Where-Object { $_.Status -eq "FAIL" }).Count
$partial = @($script:results | Where-Object { $_.Status -eq "PARTIAL" }).Count
$notDone = @($script:results | Where-Object { $_.Status -eq "NOT-DONE" }).Count

Write-Host ""
Write-Host "PASS=$pass  PARTIAL=$partial  NOT-DONE=$notDone  FAIL=$fail"

if (-not $spcPassed) { Show-Details $spc "demo-spc.ps1" }
if (-not $racePassed) { Show-Details $race "demo-stock-race.ps1" }

if ($fail -gt 0) {
    Write-Host "VERIFICATION FAILED" -ForegroundColor Red
    exit 1
}
Write-Host "No failures. Items marked PARTIAL or NOT-DONE are listed honestly in the README." -ForegroundColor Green
exit 0