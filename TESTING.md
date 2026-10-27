# Test ve Doğrulama Kılavuzu

Bu dosya, sistemi nasıl çalıştıracağınızı ve her özelliği nasıl deneyeceğinizi anlatır.

## 1. Sistemi başlatma

```powershell
Copy-Item .env.example .env      # Linux/macOS: cp .env.example .env
docker compose up --build -d --wait
```

- Arayüz: http://localhost:3000
- API: http://localhost:8080 (`/health`, `/metrics`, `/openapi.yaml`)
- Dört servis: `db`, `backend`, `frontend`, `simulator`. `docker compose ps` dördünü de `(healthy)` göstermelidir.

## 2. Demo hesaplar

Tüm hesapların parolası: `Demo!2026Parola` (`.env` içindeki `DEMO_PASSWORD`).

| Rol | E-posta |
|---|---|
| Operatör | operator@hatkontrol.test |
| Hat şefi | hatsefi@hatkontrol.test |
| Kalite mühendisi | kalite@hatkontrol.test |
| Üretim planlama | planlama@hatkontrol.test |
| Bakım | bakim@hatkontrol.test |
| Fabrika müdürü | mudur@hatkontrol.test |

## 3. Tek komutla doğrulama

```bash
make verify
# make yoksa (yalnızca Docker gerekir):
docker compose --profile verify run --rm verify
# Windows'ta doğrudan:
powershell -ExecutionPolicy Bypass -File .\scripts\verify.ps1
```

İlk çalıştırmada soy ağacı verisi yoksa 1 milyon partiyi yükler; toplam süre birkaç dakikadır. `FAIL` varsa çıkış kodu sıfırdan farklıdır. Makefile karşılığı `make verify`'dır (Windows'ta denenmedi).

| Değer | Anlamı |
|---|---|
| PASS | Çalışan sistem üzerinde denendi ve geçti |
| FAIL | Denendi ve geçmedi |
| PARTIAL | Kısmen yapıldı, eksik kısım nota yazılıdır |
| NOT-DONE | Yapılmadı |

Son alınan sonuç: `PASS=18 PARTIAL=0 NOT-DONE=0 FAIL=0` (HEALTH, AUTH-1/2/3, AUDIT, SIM, T-1 ile T-5, F-1 ile F-7).

## 4. Verileri sıfırlama

```bash
make reset-demo
# make yoksa:
docker compose --profile verify down -v
docker compose up --build -d --wait
```

**Dikkat:** Veritabanındaki tüm veriyi siler. Demo hesaplar backend açılırken, simülatör grafiği ve hattı simülatör açılırken yeniden oluşur. Sıfırlamadan sonra `verify` aynı sonucu verir.

## 5. Kimlik tablosu: nasıl denenir, beklenen sonuç

| Kimlik | Nasıl denenir | Beklenen sonuç |
|---|---|---|
| HEALTH | `GET /health` | `{"status":"ok","database":"ok"}` |
| AUTH-1 | Yanlış parola ve kayıtsız e-posta ile giriş | İkisi de 401, aynı mesaj |
| AUTH-2 | Token olmadan ve bozuk token ile `GET /me`; refresh token ile `POST /auth/refresh` | 401; yenileme 200, access token ile yenileme 401 |
| AUTH-3 | Operatörle `POST /charts` | 403 |
| AUDIT | Müdürle ve operatörle `GET /audit-log` | Müdür kayıtları kullanıcı kimliğiyle görür, operatör 403 |
| SIM | Arayüzde `SIM-SPC-42` grafiği | Noktalar 5 sn'de bir artar |
| F-1, T-4 | `scripts/demo-stock-race.ps1` | 26 PASS, `race: sent=30 ok=20 insufficient_stock=10 other=0` |
| F-2, T-2 | `scripts/demo-oee.ps1`, `go test ./internal/oee/ -v` | 17 PASS; geç arıza A'ya 60, B'ye 30 dk düşer; tekrar gelen sayaç sayılmaz |
| F-3 | `verify.ps1` F-3 adımları; arayüzde Kalite formları | Tolerans dışı değer işaretlenir, değer SPC'ye gider, süresi geçmiş kalibrasyon 409 |
| F-4, T-1 | `scripts/demo-spc.ps1`, `go test ./internal/spc/ -v` | 12 PASS |
| F-5 | `verify.ps1` NCR adımları; arayüzde Uygunsuzluklar | Bakım 403, erken kapanış 409, kök nedensiz 400, kalite kapatır 200 |
| F-6 | Arayüzde OEE panosu | `SIM-HAT-42` için A/B/C ve gün kartları, 10 sn'de bir yenilenir |
| F-7, T-3 | `GET /traceability/lots/RAW-050000/recall` (kalite) | 36 seri, 36 sevkiyat, 1 sn'nin çok altında |
| T-5 | `cd backend; go run ./cmd/sensorload` | Hata %0, p95 yaklaşık 40-50 ms |

### Kanıt komutları

```powershell
# Birim testleri ve kapsama
cd backend; go test ./... -cover; cd ..

# T-1 SPC
powershell -ExecutionPolicy Bypass -File .\scripts\demo-spc.ps1

# T-2 OEE ve geç gelen olaylar
powershell -ExecutionPolicy Bypass -File .\scripts\demo-oee.ps1

# T-3 Soy ağacı (1 milyon parti)
Get-Content .\scripts\seed-genealogy.sql | docker compose exec -T db psql -U hatkontrol -d hatkontrol
Get-Content .\scripts\measure-genealogy.sql | docker compose exec -T db psql -U hatkontrol -d hatkontrol

# T-4 İş emri ve stok yarışı
powershell -ExecutionPolicy Bypass -File .\scripts\demo-stock-race.ps1

# T-5 Yük testi (60 sn) ve arşivleme
cd backend; go run ./cmd/sensorload; cd ..
Get-Content .\scripts\demo-sensor-archive.sql | docker compose exec -T db psql -U hatkontrol -d hatkontrol

# İndeks ölçümü (README, soru 3)
Get-Content .\scripts\measure-indexes.sql | docker compose exec -T db psql -U hatkontrol -d hatkontrol

# Kaos: veritabanı kesintisi ve toparlanma
powershell -ExecutionPolicy Bypass -File .\scripts\chaos.ps1
```

## 6. Elle API denemesi

**VS Code REST Client:** `requests.http` dosyasını açın, isteklerin üstündeki **Send Request** bağlantısına tıklayın. Önce "GIRIS" bölümündeki altı giriş isteğini gönderin; sonraki istekler onların token'larını kullanır. Malzeme, ürün ve iş emri istekleri sırayla gönderilmelidir (her biri bir öncekinin kimliğini kullanır). İş emri oluşturma isteğini iki kez gönderince aynı kimlik ve `Idempotent-Replayed: true` döner.

**PowerShell:**

```powershell
$login = Invoke-RestMethod -Method Post -Uri http://localhost:8080/auth/login -ContentType "application/json" -Body '{"email":"mudur@hatkontrol.test","password":"Demo!2026Parola"}'
$headers = @{ Authorization = "Bearer $($login.access_token)" }
Invoke-RestMethod -Uri "http://localhost:8080/work-orders?limit=5" -Headers $headers
Invoke-RestMethod -Uri "http://localhost:8080/lines/SIM-HAT-42/oee?date=$((Get-Date).ToUniversalTime().ToString('yyyy-MM-dd'))" -Headers $headers | ConvertTo-Json -Depth 4
```

## 7. Arayüzde elle denenecek senaryolar

http://localhost:3000 (güncellemelerden sonra `Ctrl+F5`). Üstteki menü: SPC grafikleri, İş emirleri, OEE panosu, Kalite formları, Uygunsuzluklar, İzlenebilirlik.

1. **Giriş:** yanlış parola ile kırmızı hata; doğru parola ile sağ üstte ad ve rol.
2. **SPC (canlı):** `SIM-SPC-42` grafiğini açın. Noktalar 5 sn'de bir artar; yaklaşık 61. noktadan sonra trend başlar ve kırmızı alarm noktaları çıkar.
3. **Ölçüm girişi:** operatörle bir grafiğe `10` girin (yeşil mesaj), `abc` girin ("Geçerli bir sayı girin.").
4. **Rol görünümü:** bakım hesabıyla ölçüm formu görünmez; sunucu da 403 döner.
5. **İş emirleri:** planlama ile yeni iş emri oluşturun ve serbest bırakın; operatörle