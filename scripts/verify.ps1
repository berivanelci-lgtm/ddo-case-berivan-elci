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
Add-Result "F-3" $(if ($spcPassed) { "PARTIAL" } else { "FAIL" }) "measurement entry works through SPC charts; no separate quality form definition"
Add-Result "F-4" $(if ($spcPassed) { "PASS" } else { "FAIL" }) "control charts and alarms through API and web UI (same script as T-1)"

Add-Result "T-5" "NOT-DONE" "high-volume sensor ingestion is not implemented"
# --- Audit log: critical changes are recorded with the acting user; only the factory manager can read ---
$managerLogin = Invoke-Login "mudur@hatkontrol.test" $Password
$managerToken = ""
if ($managerLogin.Status -eq 200) { $managerToken = $managerLogin.Body.access_token }
$auditManager = Invoke-Api -Method Get -Path "/audit-log?limit=20" -Token $managerToken
$auditOperator = Invoke-Api -Method Get -Path "/audit-log?limit=20" -Token $operatorToken
$auditItems = @()
if ($auditManager.Body -and $auditManager.Body.items) { $auditItems = @($auditManager.Body.items) }
$withUser = @($auditItems | Where-Object { $null -ne $_.user_id }).Count
$auditOk = $auditManager.Status -eq 200 -and $auditItems.Count -gt 0 -and $withUser -eq $auditItems.Count -and $auditOperator.Status -eq 403
Add-Result "AUDIT" $(if ($auditOk) { "PASS" } else { "FAIL" }) "audit log has records with acting user; operator gets 403, manager reads (append-only enforced by database triggers)"
# --- T-3 and F-7: genealogy on 1,000,000 lots and recall report ---
$qualityLogin = Invoke-Login "kalite@hatkontrol.test" $Password
$qualityToken = ""
if ($qualityLogin.Status -eq 200) { $qualityToken = $qualityLogin.Body.access_token }

$probe = Invoke-Api -Method Get -Path "/traceability/lots/RAW-050000/recall" -Token $qualityToken
if ($probe.Status -eq 404) {
    Write-Host "Genealogy data not found, seeding 1,000,000 lots (about a minute)..."
    Push-Location (Split-Path -Parent $scriptDir)
    $previous = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    Get-Content (Join-Path $scriptDir "seed-genealogy.sql") | docker compose exec -T db psql -U hatkontrol -d hatkontrol -q 2>&1 | Out-Null
    $ErrorActionPreference = $previous
    Pop-Location
}

function Measure-Api {
    param([string]$Path, [string]$Token)
    [void](Invoke-Api -Method Get -Path $Path -Token $Token)
    $watch = [System.Diagnostics.Stopwatch]::StartNew()
    $result = Invoke-Api -Method Get -Path $Path -Token $Token
    $watch.Stop()
    return @{ Result = $result; Ms = [int]$watch.ElapsedMilliseconds }
}

$back = Measure-Api "/traceability/lots/SN-0300000/backward" $qualityToken
$fwd = Measure-Api "/traceability/lots/RAW-050000/forward" $qualityToken
$rec = Measure-Api "/traceability/lots/RAW-050000/recall" $qualityToken
$opTrace = Invoke-Api -Method Get -Path "/traceability/lots/RAW-050000/recall" -Token $operatorToken

$backRaw = @($back.Result.Body.lots | Where-Object { $_.kind -eq "raw" }).Count
$fwdCount = @($fwd.Result.Body.lots | Where-Object { $_ }).Count
$recShip = @($rec.Result.Body.shipments | Where-Object { $_ }).Count

$t3Ok = $back.Result.Status -eq 200 -and $backRaw -gt 0 -and $fwd.Result.Status -eq 200 -and $fwdCount -gt 0 -and $back.Ms -lt 1000 -and $fwd.Ms -lt 1000 -and $opTrace.Status -eq 403
Add-Result "T-3" $(if ($t3Ok) { "PASS" } else { "FAIL" }) "1M lots: backward $($back.Ms) ms ($backRaw raw lots), forward $($fwd.Ms) ms ($fwdCount lots), operator 403"

$f7Ok = $rec.Result.Status -eq 200 -and $recShip -gt 0 -and $rec.Ms -lt 1000
Add-Result "F-7" $(if ($f7Ok) { "PASS" } else { "FAIL" }) "recall report RAW-050000: $($rec.Result.Body.affected_serial_count) serials, $recShip shipments, $($rec.Ms) ms"
# --- F-5: NCR lifecycle (open -> investigating -> closed) ---
$maintLogin = Invoke-Login "bakim@hatkontrol.test" $Password
$maintToken = ""
if ($maintLogin.Status -eq 200) { $maintToken = $maintLogin.Body.access_token }
$ncrBody = '{"title":"verify burr on flange","description":"found during check","severity":"major"}'
$closeBody = '{"root_cause":"tool wear","corrective_action":"tool replaced"}'

$ncrMaint = Invoke-Api -Method Post -Path "/ncrs" -Token $maintToken -Body $ncrBody
$ncrNew = Invoke-Api -Method Post -Path "/ncrs" -Token $operatorToken -Body $ncrBody
$ncrId = 0
if ($ncrNew.Body) { $ncrId = $ncrNew.Body.id }
$ncrOpClose = Invoke-Api -Method Post -Path "/ncrs/$ncrId/close" -Token $operatorToken -Body $closeBody
$ncrEarly = Invoke-Api -Method Post -Path "/ncrs/$ncrId/close" -Token $qualityToken -Body $closeBody
$ncrInv = Invoke-Api -Method Post -Path "/ncrs/$ncrId/investigate" -Token $qualityToken
$ncrNoCause = Invoke-Api -Method Post -Path "/ncrs/$ncrId/close" -Token $qualityToken -Body '{"root_cause":"","corrective_action":"x"}'
$ncrClosed = Invoke-Api -Method Post -Path "/ncrs/$ncrId/close" -Token $qualityToken -Body $closeBody

$f5Ok = $ncrMaint.Status -eq 403 -and $ncrNew.Status -eq 201 -and $ncrOpClose.Status -eq 403 -and $ncrEarly.Status -eq 409 -and $ncrInv.Status -eq 200 -and $ncrNoCause.Status -eq 400 -and $ncrClosed.Status -eq 200 -and $ncrClosed.Body.status -eq "closed"
Add-Result "F-5" $(if ($f5Ok) { "PASS" } else { "FAIL" }) "NCR: maintenance 403, operator opens 201, early close 409, close without root cause 400, quality closes 200"
# --- T-2, F-2, F-6: OEE from events, late events, downtime reasons ---
$oeeRun = Invoke-ChildScript "demo-oee.ps1"
$oeePassed = $oeeRun.Code -eq 0
Add-Result "T-2" $(if ($oeePassed) { "PASS" } else { "FAIL" }) "OEE recomputed from events; late breakdown updates past shift and is split at 14:00, scripts/demo-oee.ps1"
Add-Result "F-2" $(if ($oeePassed -and $racePassed) { "PASS" } else { "FAIL" }) "counters, scrap, rework (T-4) and line counters with planned/unplanned downtime reasons (T-2)"
Add-Result "F-6" $(if ($oeePassed) { "PARTIAL" } else { "FAIL" }) "OEE by line, shift and day through the API; no UI dashboard"
if (-not $oeePassed) { foreach ($line in $oeeRun.Output) { if ($line -match "^FAIL") { Write-Host "  [demo-oee.ps1] $line" -ForegroundColor Yellow } } }
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