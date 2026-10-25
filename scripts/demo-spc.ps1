# SPC demo and verification script (T-1).
# Creates a chart, loads a 25-point baseline, then checks alarms and role rules.
param(
    [string]$BaseUrl = "http://localhost:8080",
    [string]$Password = "Demo!2026Parola"
)

$ErrorActionPreference = "Stop"
$invariant = [System.Globalization.CultureInfo]::InvariantCulture
$script:failures = 0

function Check {
    param([string]$Name, [bool]$Condition)
    if ($Condition) {
        Write-Host "PASS  $Name" -ForegroundColor Green
    } else {
        Write-Host "FAIL  $Name" -ForegroundColor Red
        $script:failures++
    }
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
    try {
        $response = Invoke-WebRequest @params
        return @{ Status = [int]$response.StatusCode; Body = ($response.Content | ConvertFrom-Json) }
    } catch {
        $status = 0
        if ($_.Exception.Response) { $status = [int]$_.Exception.Response.StatusCode }
        return @{ Status = $status; Body = $null }
    }
}

function Get-Token {
    param([string]$Email)
    $body = '{"email":"' + $Email + '","password":"' + $Password + '"}'
    $result = Invoke-Api -Method Post -Path "/auth/login" -Body $body
    if ($result.Status -ne 200) { throw "Login failed for $Email (status $($result.Status))" }
    return $result.Body.access_token
}

function Get-AlarmCount {
    param($Body)
    if ($null -eq $Body -or $null -eq $Body.alarms) { return 0 }
    return @($Body.alarms).Count
}

function Add-Value {
    param([double]$Value)
    $text = $Value.ToString($invariant)
    return Invoke-Api -Method Post -Path "/charts/$chartId/measurements" -Token $operator -Body ('{"values":[' + $text + ']}')
}

Write-Host "== SPC demo: $BaseUrl ==" -ForegroundColor Cyan

$quality = Get-Token "kalite@hatkontrol.test"
$operator = Get-Token "operator@hatkontrol.test"

# --- Authentication and roles ---
$result = Invoke-Api -Method Get -Path "/charts"
Check "request without token is rejected (401)" ($result.Status -eq 401)

$chartName = "demo-cap-" + (Get-Date -Format "yyyyMMdd-HHmmss")
$chartBody = '{"name":"' + $chartName + '","type":"individual"}'

$result = Invoke-Api -Method Post -Path "/charts" -Token $operator -Body $chartBody
Check "operator cannot create a chart (403)" ($result.Status -eq 403)

$result = Invoke-Api -Method Post -Path "/charts" -Token $quality -Body $chartBody
Check "quality engineer creates a chart (201)" ($result.Status -eq 201)
if ($result.Status -ne 201) { throw "Chart could not be created, stopping." }
$chartId = $result.Body.id

$result = Invoke-Api -Method Post -Path "/charts/$chartId/limits/recompute" -Token $operator
Check "operator cannot recompute limits (403)" ($result.Status -eq 403)

# --- Baseline: 25 points alternating 9 and 11 ---
$baselineOk = $true
$last = $null
for ($i = 0; $i -lt 25; $i++) {
    $value = if ($i % 2 -eq 0) { 9 } else { 11 }
    $last = Add-Value $value
    if ($last.Status -ne 201) { $baselineOk = $false }
}
Check "25 baseline measurements are accepted" $baselineOk
Check "limits are computed on the 25th point (version 1)" ($last.Body.limits_computed -eq $true -and $last.Body.limit_version -eq 1)
# Expected: center 9.96, sigma 2/1.128, UCL = 9.96 + 3 * 2 / 1.128 = 15.2791
Check "UCL matches the hand calculation (about 15.279)" ([math]::Abs($last.Body.limits.ucl - 15.2791) -lt 0.001)

# --- One outlier ---
$result = Add-Value 16
Check "outlier 16 raises exactly one alarm" ($result.Status -eq 201 -and (Get-AlarmCount $result.Body) -eq 1)
Check "alarm is beyond_3_sigma on point 26" ($result.Body.alarms[0].rule -eq "beyond_3_sigma" -and $result.Body.alarms[0].point_seq -eq 26)

# --- Twelve points on the same side: only one alarm, no duplicates ---
$alarmCount = 0
for ($i = 0; $i -lt 12; $i++) {
    $result = Add-Value 10.5
    $alarmCount += Get-AlarmCount $result.Body
}
Check "12 consecutive points above center raise only one alarm" ($alarmCount -eq 1)

# --- Stored result ---
$result = Invoke-Api -Method Get -Path "/charts/$chartId" -Token $operator
Check "chart detail shows both stored alarms" ($result.Status -eq 200 -and (Get-AlarmCount $result.Body) -eq 2)

# --- New limit version ---
$result = Invoke-Api -Method Post -Path "/charts/$chartId/limits/recompute" -Token $quality
Check "quality engineer creates limit version 2" ($result.Status -eq 201 -and $result.Body.version -eq 2)

Write-Host ""
if ($script:failures -eq 0) {
    Write-Host "ALL CHECKS PASSED" -ForegroundColor Green
    exit 0
}
Write-Host "$($script:failures) CHECK(S) FAILED" -ForegroundColor Red
exit 1