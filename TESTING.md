# Test ve Doğrulama Kılavuzu

Bu dosya, sistemi nasıl çalıştıracağınızı ve her özelliği nasıl deneyeceğinizi anlatır.
Yapılmayan kimlikler de açıkça listelenmiştir.

## 1. Sistemi başlatma

```powershell
Copy-Item .env.example .env      # Linux/macOS: cp .env.example .env
docker compose up --build -d --wait
```

- Arayüz: http://localhost:3000
- API: http://localhost:8080 (sağlık: `/health`, metrikler: `/metrics`)

## 2. Demo hesaplar

Tüm hesapların parolası aynıdır: `Demo!2026Parola` (`.env` içindeki `DEMO_PASSWORD`).

| Rol | E-posta |
|---|---|
| Operatör | operator@hatkontrol.test |
| Hat şefi | hatsefi@hatkontrol.test |
| Kalite mühendisi | kalite@hatkontrol.test |
| Üretim planlama | planlama@hatkontrol.test |
| Bakım | bakim@hatkontrol.test |
| Fabrika müdürü | mudur@hatkontrol.test |

## 3. Tek komutla doğrulama

Sistem ayaktayken (Windows PowerShell, proje kök klasöründe):

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\verify.ps1
```

İlk çalıştırmada soy ağacı verisi yoksa 1 milyon partiyi yükler; toplam süre birkaç dakikadır. Her kimlik için sonuç tablosu basar; `FAIL` varsa çıkış kodu sıfırdan farklıdır. Makefile karşılığı `make verify`'dır (Windows'ta denenmedi).

| Değer | Anlamı |
|---|---|
| PASS | Çalışan sistem üzerinde denendi ve geçti |
| FAIL | Denendi ve geçmedi |
| PARTIAL | Kısmen yapıldı, eksik kısım nota yazılıdır |
| NOT-DONE | Yapılmadı |

Son alınan sonuç:

```
HEALTH   PASS      GET /health returns 200 with database ok
AUTH-1   PASS      wrong password and unknown e-mail both return 401
AUTH-2   PASS      valid login 200, missing or invalid token 401
AUTH-3   PASS      operator creating a chart is rejected with 403
T-1      PASS      SPC rule engine, scripts/demo-spc.ps1 (12 checks)
T-4      PASS      work order state machine and stock race, scripts/demo-stock-race.ps1 (26 checks)
F-1      PASS      work orders and BOM through the API (same script as T-4)
F-3      PARTIAL   measurement entry works through SPC charts; no separate quality form definition
F-4      PASS      control charts and alarms through API and web UI (same script as T-1)
T-5      NOT-DONE  high-volume sensor ingestion is not implemented
AUDIT    PASS      audit log has records with acting user; operator gets 403, manager reads
T-3      PASS      1M lots: backward 27 ms (6 raw lots), forward 26 ms (45 lots), operator 403
F-7      PASS      recall report RAW-050000: 36 serials, 36 shipments, 29 ms
F-5      PASS      NCR: maintenance 403, operator opens 201, early close 409, close without root cause 400, quality closes 200
T-2      PASS      OEE recomputed from events; late breakdown updates past shift and is split at 14:00
F-2      PASS      counters, scrap, rework (T-4) and line counters with planned/unplanned downtime reasons (T-2)
F-6      PARTIAL   OEE by line, shift and day through the API; no UI dashboard

PASS=14  PARTIAL=2  NOT-DONE=1  FAIL=0
```

## 4. Verileri sıfırlama

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\reset-demo.ps1
```

**Dikkat:** Veritabanındaki tüm veriyi siler. Demo hesaplar sistem açılırken yeniden eklenir. Sıfırlamadan sonra `verify` aynı sonucu verir (soy ağacı verisini yeniden yükler).

## 5. Kimlik tablosu: nasıl denenir, beklenen sonuç

| Kimlik | Durum | Nasıl denenir | Beklenen sonuç |
|---|---|---|---|
| HEALTH | Tamam | `GET /health` | `{"status":"ok","database":"ok"}` |
| AUTH-1 | Tamam | Yanlış parola ve kayıtsız e-posta ile `POST /auth/login` | İkisi de 401, aynı mesaj |
| AUTH-2 | Tamam | Token olmadan ve bozuk token ile `GET /me` | 401 |
| AUTH-3 | Tamam | Operatör token'ı ile `POST /charts` | 403 |
| AUDIT | Tamam | Müdürle `GET /audit-log`, operatörle aynı istek | Müdür kayıtları görür (kullanıcı kimliğiyle), operatör 403 |
| F-1 | Tamam | `scripts/demo-stock-race.ps1` | `ALL CHECKS PASSED` |
| F-2 | Tamam | `scripts/demo-stock-race.ps1` ve `scripts/demo-oee.ps1` | İş emri sayaçları tutarlı; duruş nedenleri planlı/plansız ayrılır |
| F-3 | Kısmen | SPC grafiğine ölçüm girişi (API ve arayüz) | **Ayrı kalite formu tanımı yok** |
| F-4 | Tamam | `scripts/demo-spc.ps1` ve arayüz (bölüm 7) | 12 PASS |
| F-5 | Tamam | `verify.ps1` içindeki NCR adımları | Bakım 403, erken kapanış 409, kök nedensiz 400, kalite kapatır 200 |
| F-6 | Kısmen | `GET /lines/{code}/oee?date=...` | Vardiya ve gün değerleri; **arayüz panosu yok** |
| F-7 | Tamam | `GET /traceability/lots/RAW-050000/recall` (kalite) | 36 seri, 36 sevkiyat |
| T-1 | Tamam | `scripts/demo-spc.ps1`, `go test ./internal/spc/ -v` | 12 PASS |
| T-2 | Tamam | `scripts/demo-oee.ps1`, `go test ./internal/oee/ -v` | 16 PASS; geç arıza A'ya 60, B'ye 30 dk düşer |
| T-3 | Tamam | `verify.ps1` (soy ağacı adımları), `scripts/measure-genealogy.sql` | 1M partide 1 sn'nin çok altında |
| T-4 | Tamam | `scripts/demo-stock-race.ps1` | `race: sent=30 ok=20 insufficient_stock=10 other=0` |
| T-5 | Yapılmadı | | |

### Kanıt komutları (zorlu görevler)

```powershell
# T-1 SPC
cd backend; go test ./internal/spc/ -v; cd ..
powershell -ExecutionPolicy Bypass -File .\scripts\demo-spc.ps1

# T-2 OEE ve geç gelen olaylar
cd backend; go test ./internal/oee/ -v; cd ..
powershell -ExecutionPolicy Bypass -File .\scripts\demo-oee.ps1

# T-3 Soy ağacı (1 milyon parti)
Get-Content .\scripts\seed-genealogy.sql | docker compose exec -T db psql -U hatkontrol -d hatkontrol
Get-Content .\scripts\measure-genealogy.sql | docker compose exec -T db psql -U hatkontrol -d hatkontrol

# T-4 İş emri ve stok yarışı
cd backend; go test ./internal/workorder/ -v; cd ..
powershell -ExecutionPolicy Bypass -File .\scripts\demo-stock-race.ps1

# İndeks ölçümü (README, Doğrulama Soruları 3)
Get-Content .\scripts\measure-indexes.sql | docker compose exec -T db psql -U hatkontrol -d hatkontrol

# Kaos: veritabanı kesintisi ve toparlanma
powershell -ExecutionPolicy Bypass -File .\scripts\chaos.ps1
```

Tüm backend testleri ve paket bazında kapsama:

```powershell
cd backend
go test ./... -cover
```

## 6. Elle API denemesi (PowerShell)

```powershell
$body = @{ email = "mudur@hatkontrol.test"; password = "Demo!2026Parola" } | ConvertTo-Json
$login = Invoke-RestMethod -Method Post -Uri http://localhost:8080/auth/login -ContentType "application/json" -Body $body
$headers = @{ Authorization = "Bearer $($login.access_token)" }

Invoke-RestMethod -Uri "http://localhost:8080/work-orders?limit=5" -Headers $headers
Invoke-RestMethod -Uri "http://localhost:8080/ncrs?status=open" -Headers $headers
Invoke-RestMethod -Uri http://localhost:8080/lines -Headers $headers
Invoke-RestMethod -Uri "http://localhost:8080/traceability/lots/RAW-050000/recall" -Headers $headers | ConvertTo-Json -Depth 4
Invoke-RestMethod -Uri "http://localhost:8080/audit-log?limit=5" -Headers $headers | ConvertTo-Json -Depth 5
```

OEE raporu için önce `scripts/demo-oee.ps1` bir hat oluşturur (`OEE-<zaman>`); `GET /lines` ile kodunu alıp `GET /lines/<kod>/oee?date=2026-10-20` çağrılabilir.

`requests.http` (VS Code REST Client) dosyası giriş, SPC, katalog ve iş emri akışını içerir; **ben çalıştırıp denemedim** ve NCR, OEE, izlenebilirlik uçları içinde yok.

## 7. Arayüzde elle denenecek senaryolar

http://localhost:3000 adresinde:

1. **Giriş hatası:** `operator@hatkontrol.test` ve yanlış parola. Kırmızı hata mesajı çıkmalı.
2. **Giriş:** Parola `Demo!2026Parola`. Sağ üstte `Demo Operatör (Operatör)` görünmeli. Grafik yoksa "Henüz kontrol grafiği yok." yazar.
3. **Grafik oluşturma:** Arayüzde form yoktur; `scripts/demo-spc.ps1` bir grafik oluşturur. Sonra sayfayı yenileyip grafiğe tıklayın.
4. **Grafik ekranı:** Noktalar, kırmızı kesikli UCL/LCL ve merkez çizgisi; alarm veren noktalar kırmızı ve büyük.
5. **Normal ölçüm:** `10` girin, yeşil "Ölçüm #... eklendi." mesajı.
6. **Alarm:** `20` girin, kırmızı "1 alarm üretildi." mesajı ve alarm listesine yeni satır.
7. **Geçersiz giriş:** `abc` girin, "Geçerli bir sayı girin." yazmalı.
8. **Rol görünümü:** Bakım hesabıyla ölçüm formu görünmez; kalite hesabıyla form ve "Limitleri yeniden hesapla" butonu görünür.
9. **Sunucu tarafı yetki:** Bakım token'ı ile `POST /charts/1/measurements` 403 döner.
10. **Hata durumu:** `docker compose stop backend`, sayfayı yenileyin, hata mesajı görünmeli; sonra `docker compose start backend`.
11. **Dar ekran:** F12, telefon simgesi; grafik yatay kaydırılabilir, sayfa taşmamalı.

Arayüzde **iş emri, katalog, OEE, NCR ve izlenebilirlik ekranları yoktur**; bunlar yalnızca API üzerinden denenir.