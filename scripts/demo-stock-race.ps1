# T-4 proof script: stock race across work orders, scrap, rework, partial close, keyset paging.
# ASCII only on purpose (Windows PowerShell reads non-ASCII script files unreliably).
param(
    [string]$BaseUrl = "http://localhost:8080",
    [string]$Password = "Demo!2026Parola",
    [int]$Orders = 6,
    [int]$Requests = 30,
    [int]$Stock = 100,
    [int]$QtyPerUnit = 5
)

$ErrorActionPreference = "Stop"
# Must be set before the first request, otherwise .NET keeps only 2 connections per host.
[System.Net.ServicePointManager]::DefaultConnectionLimit = 100
Add-Type -AssemblyName System.Net.Http
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

    $status = 0
    $text = ""
    try {
        $response = Invoke-WebRequest @params
        $status = [int]$response.StatusCode
        $text = $response.Content
        } catch {
        if ($_.Exception.Response) {
            $status = [int]$_.Exception.Response.StatusCode
            if ($_.ErrorDetails -and $_.ErrorDetails.Message) {
                $text = $_.ErrorDetails.Message
            } else {
                try {
                    $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
                    $text = $reader.ReadToEnd()
                    $reader.Close()
                } catch {
                    $text = ""
                }
            }
        }
    }
    $parsed = $null
    if ($text) {
        try { $parsed = $text | ConvertFrom-Json } catch { $parsed = $null }
    }
    return @{ Status = $status; Body = $parsed }
}

function Get-ErrorCode {
    param($Result)
    if ($Result.Body -and $Result.Body.error) { return [string]$Result.Body.error.code }
    return ""
}

function Get-Token {
    param([string]$Email)
    $body = '{"email":"' + $Email + '","password":"' + $Password + '"}'
    $result = Invoke-Api -Method Post -Path "/auth/login" -Body $body
    if ($result.Status -ne 200) { throw "Login failed for $Email (status $($result.Status))" }
    return $result.Body.access_token
}

# Sends all requests at (almost) the same time and returns status and error code of each.
function Send-ParallelPosts {
    param([string[]]$Paths, [string]$Token, [string]$Body)

    $client = New-Object System.Net.Http.HttpClient
    $client.Timeout = [TimeSpan]::FromSeconds(120)
    $tasks = New-Object System.Collections.ArrayList
    foreach ($path in $Paths) {
        $request = New-Object System.Net.Http.HttpRequestMessage -ArgumentList @([System.Net.Http.HttpMethod]::Post, "$BaseUrl$path")
        [void]$request.Headers.TryAddWithoutValidation("Authorization", "Bearer $Token")
        $request.Content = New-Object System.Net.Http.StringContent -ArgumentList @($Body, [System.Text.Encoding]::UTF8, "application/json")
        [void]$tasks.Add($client.SendAsync($request))
    }
    $taskArray = [System.Threading.Tasks.Task[]]($tasks.ToArray())
    [void][System.Threading.Tasks.Task]::WaitAll($taskArray)

    $results = @()
    foreach ($task in $tasks) {
        $response = $task.Result
        $text = $response.Content.ReadAsStringAsync().Result
        $code = ""
        try { $code = [string](($text | ConvertFrom-Json).error.code) } catch { $code = "" }
        $results += [pscustomobject]@{ Status = [int]$response.StatusCode; Code = $code }
    }
    $client.Dispose()
    return $results
}

function New-Order {
    param([long]$ProductId, [int]$Planned)
    $created = Invoke-Api -Method Post -Path "/work-orders" -Token $planner -Body ('{"product_id":' + $ProductId + ',"planned_qty":' + $Planned + '}')
    if ($created.Status -ne 201) { throw "Work order could not be created (status $($created.Status))" }
    return [long]$created.Body.id
}

function Release-Order {
    param([long]$OrderId)
    $released = Invoke-Api -Method Post -Path "/work-orders/$OrderId/release" -Token $planner
    if ($released.Status -ne 200) { throw "Work order $OrderId could not be released (status $($released.Status))" }
}

function Get-Order {
    param([long]$OrderId)
    return (Invoke-Api -Method Get -Path "/work-orders/$OrderId" -Token $planner).Body
}

function Get-MaterialStock {
    param([long]$MaterialId)
    $materials = Invoke-Api -Method Get -Path "/materials" -Token $planner
    $row = @($materials.Body | Where-Object { $_.id -eq $MaterialId })[0]
    return [long]$row.stock_qty
}

Write-Host "== T-4 stock race and work order demo: $BaseUrl ==" -ForegroundColor Cyan

$planner = Get-Token "planlama@hatkontrol.test"
$operator = Get-Token "operator@hatkontrol.test"
$supervisor = Get-Token "hatsefi@hatkontrol.test"
$maintenance = Get-Token "bakim@hatkontrol.test"
$stamp = Get-Date -Format "yyyyMMdd-HHmmss"

# ---------------------------------------------------------------
# Part A: many work orders compete for the same material stock
# ---------------------------------------------------------------
Write-Host ""
Write-Host "-- Part A: stock race --" -ForegroundColor Cyan

$result = Invoke-Api -Method Post -Path "/materials" -Token $planner -Body ('{"code":"RACE-M-' + $stamp + '","name":"Race material","stock_qty":' + $Stock + '}')
Check "planner creates the material (201)" ($result.Status -eq 201)
if ($result.Status -ne 201) { throw "Material could not be created, stopping." }
$materialId = [long]$result.Body.id

$result = Invoke-Api -Method Post -Path "/products" -Token $planner -Body ('{"code":"RACE-P-' + $stamp + '","name":"Race product","bom":[{"material_id":' + $materialId + ',"qty_per_unit":' + $QtyPerUnit + '}]}')
Check "planner creates the product with its BOM (201)" ($result.Status -eq 201)
if ($result.Status -ne 201) { throw "Product could not be created, stopping." }
$productId = [long]$result.Body.id

$result = Invoke-Api -Method Post -Path "/work-orders" -Token $operator -Body ('{"product_id":' + $productId + ',"planned_qty":100}')
Check "operator cannot create a work order (403)" ($result.Status -eq 403)

$draftId = New-Order -ProductId $productId -Planned 100
$result = Invoke-Api -Method Post -Path "/work-orders/$draftId/production" -Token $operator -Body '{"good":1}'
Check "production on a work order that is not released is rejected (409)" ($result.Status -eq 409 -and (Get-ErrorCode $result) -eq "invalid_transition")

$orderIds = @()
for ($i = 0; $i -lt $Orders; $i++) {
    $id = New-Order -ProductId $productId -Planned 100
    Release-Order -OrderId $id
    $orderIds += $id
}
Check "$Orders work orders are created and released" ($orderIds.Count -eq $Orders)

$result = Invoke-Api -Method Post -Path "/work-orders/$($orderIds[0])/production" -Token $maintenance -Body '{"good":1}'
Check "maintenance cannot record production (403)" ($result.Status -eq 403)

$paths = @()
for ($i = 0; $i -lt $Requests; $i++) {
    $paths += "/work-orders/$($orderIds[$i % $Orders])/production"
}
$responses = @(Send-ParallelPosts -Paths $paths -Token $operator -Body '{"good":1}')

$ok = @($responses | Where-Object { $_.Status -eq 200 }).Count
$noStock = @($responses | Where-Object { $_.Status -eq 409 -and $_.Code -eq "insufficient_stock" }).Count
$other = $Requests - $ok - $noStock
Write-Host "  race: sent=$Requests ok=$ok insufficient_stock=$noStock other=$other"

$expectedOk = [int][math]::Min($Requests, [math]::Floor($Stock / $QtyPerUnit))
$expectedRemaining = $Stock - ($expectedOk * $QtyPerUnit)

Check "exactly $expectedOk requests succeed (stock $Stock, $QtyPerUnit per unit)" ($ok -eq $expectedOk)
Check "all remaining requests are rejected with insufficient_stock" ($noStock -eq ($Requests - $expectedOk))
Check "no other status codes (no 500, no deadlock)" ($other -eq 0)

$stockNow = Get-MaterialStock -MaterialId $materialId
Check "material stock is $expectedRemaining and never negative" ($stockNow -eq $expectedRemaining -and $stockNow -ge 0)

$sumStarted = 0
$sumGood = 0
$startedIds = @()
foreach ($id in $orderIds) {
    $order = Get-Order -OrderId $id
    $sumStarted += [long]$order.started_qty
    $sumGood += [long]$order.good_qty
    if ($order.status -eq "in_progress") { $startedIds += $id }
}
Check "started and good totals across orders equal the successful requests" ($sumStarted -eq $expectedOk -and $sumGood -eq $expectedOk)

if ($expectedRemaining -lt $QtyPerUnit) {
    $result = Invoke-Api -Method Post -Path "/work-orders/$($orderIds[0])/production" -Token $operator -Body '{"good":1}'
    Check "one more request on depleted stock is rejected (409 insufficient_stock)" ($result.Status -eq 409 -and (Get-ErrorCode $result) -eq "insufficient_stock")
}

# Closing runs the reconciliation: consumed material must equal started units x BOM quantity.
$allClosed = $true
foreach ($id in $startedIds) {
    $result = Invoke-Api -Method Post -Path "/work-orders/$id/close" -Token $supervisor -Body '{"allow_partial":true}'
    if ($result.Status -ne 200 -or $result.Body.status -ne "completed") { $allClosed = $false }
}
Check "every started work order closes and passes the material reconciliation (200)" ($allClosed -and $startedIds.Count -gt 0)

$result = Invoke-Api -Method Post -Path "/work-orders/$($startedIds[0])/production" -Token $operator -Body '{"good":1}'
Check "a completed work order rejects further production (409)" ($result.Status -eq 409 -and (Get-ErrorCode $result) -eq "invalid_transition")

# ---------------------------------------------------------------
# Part B: scrap, rework loop and partial completion
# ---------------------------------------------------------------
Write-Host ""
Write-Host "-- Part B: scrap, rework, partial completion --" -ForegroundColor Cyan

$result = Invoke-Api -Method Post -Path "/materials" -Token $planner -Body ('{"code":"REWORK-M-' + $stamp + '","name":"Rework material","stock_qty":1000}')
$material2 = [long]$result.Body.id
$result = Invoke-Api -Method Post -Path "/products" -Token $planner -Body ('{"code":"REWORK-P-' + $stamp + '","name":"Rework product","bom":[{"material_id":' + $material2 + ',"qty_per_unit":2}]}')
$product2 = [long]$result.Body.id
$order2 = New-Order -ProductId $product2 -Planned 10
Release-Order -OrderId $order2
Check "rework scenario is set up (material, product, released order)" ($material2 -gt 0 -and $product2 -gt 0 -and $order2 -gt 0)

# 10 units start: 6 good, 1 scrap, 3 go to rework. Material used: 10 x 2 = 20.
$result = Invoke-Api -Method Post -Path "/work-orders/$order2/production" -Token $operator -Body '{"good":6,"scrap":1,"rework":3}'
Check "production with scrap and rework updates the counters" ($result.Status -eq 200 -and $result.Body.started_qty -eq 10 -and $result.Body.good_qty -eq 6 -and $result.Body.scrap_qty -eq 1 -and $result.Body.rework_pending -eq 3)

$result = Invoke-Api -Method Post -Path "/work-orders/$order2/production" -Token $operator -Body '{"good":2}'
Check "production beyond the plan is rejected (409 exceeds_plan)" ($result.Status -eq 409 -and (Get-ErrorCode $result) -eq "exceeds_plan")

$result = Invoke-Api -Method Post -Path "/work-orders/$order2/close" -Token $supervisor -Body '{"allow_partial":true}'
Check "close is blocked while rework is pending (409 rework_pending)" ($result.Status -eq 409 -and (Get-ErrorCode $result) -eq "rework_pending")

$result = Invoke-Api -Method Post -Path "/work-orders/$order2/rework" -Token $operator -Body '{"good":2,"scrap":1}'
Check "rework result is recorded (good 8, scrap 2, pending 0)" ($result.Status -eq 200 -and $result.Body.good_qty -eq 8 -and $result.Body.scrap_qty -eq 2 -and $result.Body.rework_pending -eq 0)

$result = Invoke-Api -Method Post -Path "/work-orders/$order2/rework" -Token $operator -Body '{"good":1}'
Check "rework beyond the pending amount is rejected (400)" ($result.Status -eq 400 -and (Get-ErrorCode $result) -eq "invalid_quantity")

$result = Invoke-Api -Method Post -Path "/work-orders/$order2/close" -Token $supervisor
Check "partial completion without approval is blocked (409 partial_not_confirmed)" ($result.Status -eq 409 -and (Get-ErrorCode $result) -eq "partial_not_confirmed")

$result = Invoke-Api -Method Post -Path "/work-orders/$order2/close" -Token $supervisor -Body '{"allow_partial":true}'
Check "partial completion with approval closes the order (200)" ($result.Status -eq 200 -and $result.Body.status -eq "completed")

$stock2 = Get-MaterialStock -MaterialId $material2
Check "material consumption matches the BOM (stock 1000 - 20 = 980)" ($stock2 -eq 980)

$final = Get-Order -OrderId $order2
Check "final counters are consistent (started = good + scrap)" ($final.started_qty -eq ($final.good_qty + $final.scrap_qty) -and $final.rework_pending -eq 0)

# ---------------------------------------------------------------
# Keyset paging
# ---------------------------------------------------------------
Write-Host ""
Write-Host "-- Keyset paging --" -ForegroundColor Cyan

$page1 = Invoke-Api -Method Get -Path "/work-orders?limit=2" -Token $planner
Check "first page has 2 items and a cursor" ($page1.Status -eq 200 -and @($page1.Body.items).Count -eq 2 -and $null -ne $page1.Body.next_before)

$cursor = $page1.Body.next_before
$page2 = Invoke-Api -Method Get -Path "/work-orders?limit=2&before=$cursor" -Token $planner
$olderOnly = $page2.Status -eq 200 -and @($page2.Body.items).Count -gt 0
foreach ($item in @($page2.Body.items)) { if ([long]$item.id -ge [long]$cursor) { $olderOnly = $false } }
Check "second page only contains older work orders" $olderOnly

Write-Host ""
if ($script:failures -eq 0) {
    Write-Host "ALL CHECKS PASSED" -ForegroundColor Green
    exit 0
}
Write-Host "$($script:failures) CHECK(S) FAILED" -ForegroundColor Red
exit 1