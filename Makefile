# Linux, macOS ve Windows'ta yalnizca make ve Docker ile ayni sekilde calisir.
# make yoksa: docker compose --profile verify run --rm verify
.PHONY: up verify reset-demo chaos load

ifeq ($(OS),Windows_NT)
POWERSHELL ?= powershell
else
POWERSHELL ?= pwsh
endif

up:
	docker compose up --build -d --wait

verify:
	docker compose --profile verify run --rm verify

reset-demo:
	docker compose --profile verify down -v
	docker compose up --build -d --wait

chaos:
	$(POWERSHELL) -NoProfile -ExecutionPolicy Bypass -File scripts/chaos.ps1

load:
	cd backend && go run ./cmd/sensorload
