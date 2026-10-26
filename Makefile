# Windows kullanıcıları make yerine betikleri doğrudan çalıştırabilir:
#   powershell -ExecutionPolicy Bypass -File .\scripts\verify.ps1
.RECIPEPREFIX = >
POWERSHELL ?= powershell
.PHONY: up verify reset-demo chaos
up:
> docker compose up --build -d

verify:
> $(POWERSHELL) -NoProfile -ExecutionPolicy Bypass -File scripts/verify.ps1

reset-demo:
> $(POWERSHELL) -NoProfile -ExecutionPolicy Bypass -File scripts/reset-demo.ps1
chaos:
> $(POWERSHELL) -NoProfile -ExecutionPolicy Bypass -File scripts/chaos.ps1