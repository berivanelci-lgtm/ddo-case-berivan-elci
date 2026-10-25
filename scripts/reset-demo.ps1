# Resets the demo to its first seeded state. WARNING: deletes ALL database data.
$ErrorActionPreference = "Stop"
Set-Location (Split-Path -Parent $PSScriptRoot)

docker compose down -v
if ($LASTEXITCODE -ne 0) { throw "docker compose down failed" }

docker compose up --build -d --wait
if ($LASTEXITCODE -ne 0) { throw "docker compose up failed" }

Write-Host "Demo reset done: empty database, demo accounts seeded again." -ForegroundColor Green