# T-2 proof: OEE per shift is recomputed from events; a late downtime event updates a past shift
# and is split across the shift boundary. Expected values are hand calculated. ASCII only.
param(
    [string]$BaseUrl = "http://localhost:8080",
    [string]$Password = "Demo!2026Parola"
)

$ErrorActionPreference = "Stop"
$script:failures = 0

function Check {
    param([string]$Name, [bool]$Condition)
    if ($Condition) { Write-Host "PASS  $Name" -ForegroundColor Green }
    else { Write-Host "FAIL  $Name" -ForegroundColor Red; $script:failures++ }
}

function Invoke-Api {
    param([string]$Method, [string]$Path, [string]$Token, [string]$Body)
    $headers = @{}
    if ($Token) { $headers["Authorization"] = "Bearer $Token" }
    $params = @{ Method = $Method; Uri = "$BaseUrl$Path"; Headers = $headers; UseBasicParsing = $true }
    if ($Body) { $params["Body"] = $Body; $params["ContentType"] = "application/json" }
    $status = 0; $text = ""
    try {
        $response = Invoke-WebRequest @params
        $status = [int]$response.StatusCode; $text = $response.Content
    } catch {
        if ($_.Exception.Response) {
            $status = [int]$_.Exception.Response.StatusCode
            if ($_.ErrorDetails -and $_.ErrorDetails.Message) { $text = $_.ErrorDetails.Message }
        }
    }
    $parsed = $null
    if ($text) { try { $parsed = $text | ConvertFrom-Json } catch { $parsed = $null } }
    return @{ Status = $status; Body = $parsed }
}

function Get-Token {
    param([string]$Email)
    $r = Invoke-Api -Method Post -Path "/auth/login" -Body ('{"email":"' + $Email + '","password":"' + $Password + '"}')
    if ($r.Status -ne 200) { throw "Login failed for $Email" }
    return $r.Body.access_token
}

function Near { param([double]$Got, [double]$Want) return [math]::Abs($Got - $Want) -lt 0.0006 }

function Get-Shift { param($Report, [string]$Name) return @($Report.Body.shifts | Where-Object { $_.shift -eq $Name })[0] }

Write-Host "== T-2 OEE and late events: $BaseUrl ==" -ForegroundColor Cyan

$planner = Get-Token "planlama@hatkontrol.test"
$operator = Get-Token "operator@hatkontrol.test"
$maintenance = Get-Token "bakim@hatkontrol.test"
$manager = Get-Token "mudur@hatkontrol.test"
$line = "OEE-" + (Get-Date -Format "yyyyMMddHHmmss")
$day = "2026-10-20"

$r = Invoke-Api -Method Post -Path "/lines" -Token $operator -Body ('{"code":"' + $line + '","name":"x","ideal_cycle_ms":60000}')
Check "operator cannot create a line (403)" ($r.Status -eq 403)
$r = Invoke-Api -Method Post -Path "/lines" -Token $planner -Body ('{"code":"' + $line + '","name":"Demo line","ideal_cycle_ms":60000}')
Check "planner creates a line with ideal cycle 1 min (201)" ($r.Status -eq 201)
if ($r.Status -ne 201) { throw "Line could not be created" }

$r = Invoke-Api -Method Get -Path "/downtime-reasons" -Token $operator
$break = @($r.Body | Where-Object { $_.code -eq "BREAK" })[0]
$breakdown = @($r.Body | Where-Object { $_.code -eq "BREAKDOWN" })[0]
Check "downtime reasons list planned BREAK and unplanned BREAKDOWN" ($break.planned -eq $true -and $breakdown.planned -eq $false)

# Shift A (06-14 UTC): planned break 10:00-10:30, 300 units (285 good). Shift B (14-22): 400 units (380 good).
$ok = $true
$r = Invoke-Api -Method Post -Path "/lines/$line/downtime" -Token $operator -Body ('{"reason_code":"BREAK","started_at":"' + $day + 'T10:00:00Z","ended_at":"' + $day + 'T10:30:00Z"}')
if ($r.Status -ne 201) { $ok = $false }
foreach ($c in @(@("08:00", 150, 5), @("12:00", 135, 10), @("18:00", 380, 20))) {
    $r = Invoke-Api -Method Post -Path "/lines/$line/counts" -Token $operator -Body ('{"occurred_at":"' + $day + 'T' + $c[0] + ':00Z","good":' + $c[1] + ',"scrap":' + $c[2] + '}')
    if ($r.Status -ne 201) { $ok = $false }
}
Check "break and three counters are recorded (201)" $ok

$before = Invoke-Api -Method Get -Path "/lines/$line/oee?date=$day" -Token $manager
$a = Get-Shift $before "A"; $b = Get-Shift $before "B"
Check "before: shift A availability 1, performance 0.6667, quality 0.95, OEE 0.6333" ((Near $a.availability 1) -and (Near $a.performance 0.6667) -and (Near $a.quality 0.95) -and (Near $a.oee 0.6333) -and $a.planned_downtime_min -eq 30)
Check "before: shift B performance 0.8333, OEE 0.7917" ((Near $b.performance 0.8333) -and (Near $b.oee 0.7917))

# Late event: a breakdown 13:00-14:30 is recorded only now, after shift A has ended. It crosses 14:00.
$r = Invoke-Api -Method Post -Path "/lines/$line/downtime" -Token $maintenance -Body ('{"reason_code":"BREAKDOWN","started_at":"' + $day + 'T13:00:00Z","ended_at":"' + $day + 'T14:30:00Z"}')
Check "maintenance records the late breakdown (201)" ($r.Status -eq 201)

$after = Invoke-Api -Method Get -Path "/lines/$line/oee?date=$day" -Token $manager
$a = Get-Shift $after "A"; $b = Get-Shift $after "B"
Check "after: shift A gets 60 min unplanned downtime (split at 14:00)" ($a.unplanned_downtime_min -eq 60)
Check "after: shift B gets the remaining 30 min" ($b.unplanned_downtime_min -eq 30)
Check "after: shift A availability 0.8667, performance 0.7692 (history updated)" ((Near $a.availability 0.8667) -and (Near $a.performance 0.7692))
Check "after: shift B availability 0.9375, performance 0.8889" ((Near $b.availability 0.9375) -and (Near $b.performance 0.8889))
Check "after: day total planned 30, unplanned 90, availability 0.9362, OEE 0.4716" ($after.Body.day.planned_downtime_min -eq 30 -and $after.Body.day.unplanned_downtime_min -eq 90 -and (Near $after.Body.day.availability 0.9362) -and (Near $after.Body.day.oee 0.4716))

$again = Invoke-Api -Method Get -Path "/lines/$line/oee?date=$day" -Token $manager
Check "same events give the same result (deterministic)" (($again.Body | ConvertTo-Json -Depth 5) -eq ($after.Body | ConvertTo-Json -Depth 5))

$future = (Get-Date).ToUniversalTime().AddHours(2).ToString("yyyy-MM-ddTHH:mm:ssZ")
$r = Invoke-Api -Method Post -Path "/lines/$line/downtime" -Token $operator -Body ('{"reason_code":"BREAKDOWN","started_at":"' + $day + 'T15:00:00Z","ended_at":"' + $future + '"}')
Check "downtime ending in the future is rejected (400)" ($r.Status -eq 400)
$r = Invoke-Api -Method Post -Path "/lines/$line/downtime" -Token $operator -Body ('{"reason_code":"NOPE","started_at":"' + $day + 'T15:00:00Z","ended_at":"' + $day + 'T15:10:00Z"}')
Check "unknown downtime reason is rejected (422)" ($r.Status -eq 422)
$r = Invoke-Api -Method Post -Path "/lines/$line/counts" -Token $maintenance -Body ('{"occurred_at":"' + $day + 'T15:00:00Z","good":1,"scrap":0}')
Check "maintenance cannot record counts (403)" ($r.Status -eq 403)

Write-Host ""
if ($script:failures -eq 0) { Write-Host "ALL CHECKS PASSED" -ForegroundColor Green; exit 0 }
Write-Host "$($script:failures) CHECK(S) FAILED" -ForegroundColor Red
exit 1