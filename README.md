# HatKontrol: Üretim Hattı Takip ve Kalite Kontrol Sistemi (MES)

**Hazırlayan:** Berivan Elçi
Bandırma Onyedi Eylül Üniversitesi, Yazılım Mühendisliği öğrencisi

Bu depo, DDO Dijital Dönüşüm Ofisi teknik değerlendirmesi için hazırlanmıştır.
Proje tamamlanmış değildir; aşağıdaki **Durum** bölümü neyin çalıştığını ve neyin yapılmadığını açıkça gösterir.

## Kurulum

Gereken araçlar: Git, Docker Desktop (Compose dahil). Go yalnızca testleri yerelde çalıştırmak için gerekir.

```powershell
git clone https://github.com/berivanelci-lgtm/ddo-case-berivan-elci.git
cd ddo-case-berivan-elci
Copy-Item .env.example .env
docker compose up --build
```

Linux ve macOS için `.env` kopyası: `cp .env.example .env`

- Backend (Go API): http://localhost:8080
- Sağlık kontrolü: http://localhost:8080/health (veritabanı bağlantısını da denetler)
- Veritabanı (PostgreSQL 16) ana makineye açılmaz, yalnızca Compose ağında erişilir.
- Şema migrasyonları ve demo hesaplar backend açılırken otomatik uygulanır, elle adım gerekmez.

## Demo hesaplar

Tüm hesapların parolası `.env.example` içindeki `DEMO_PASSWORD` değeridir: `Demo!2026Parola`
Bu değerler yalnızca demo içindir, gerçek bir ortamda değiştirilmelidir.

| Rol | E-posta |
|---|---|
| Operatör | operator@hatkontrol.test |
| Hat şefi | hatsefi@hatkontrol.test |
| Kalite mühendisi | kalite@hatkontrol.test |
| Üretim planlama | planlama@hatkontrol.test |
| Bakım | bakim@hatkontrol.test |
| Fabrika müdürü | mudur@hatkontrol.test |

## Mimari özeti

```
backend/
  cmd/api/            giriş noktası: ayarlar, bağlantı, yönlendirme, düzgün kapanış
  internal/config/    ortam değişkenlerinden ayar okuma
  internal/database/  PostgreSQL bağlantısı ve migrasyonların uygulanması
  internal/auth/      bcrypt parola özeti, JWT (access + refresh), giriş servisi
  internal/user/      roller, kullanıcı kaydı, demo hesap seed'i
  internal/spc/       SPC saf mantığı: limitler, kural motoru, ölçüm servisi
  internal/spcstore/  SPC'nin PostgreSQL deposu
  internal/httpapi/   HTTP uçları, kimlik doğrulama ve rol kontrolü ara katmanları
  migrations/         goose SQL migrasyonları (programın içine gömülür)
scripts/              uçtan uca doğrulama betikleri
```

Katmanlar arası bağımlılık arayüzlerle kurulmuştur. Bu sayede iş mantığı (`auth`, `spc`) veritabanı olmadan, sahte depolarla test edilebilir.

## Karar gerekçeleri

- **Go standart kütüphanesi (`net/http`):** Go 1.22 ile gelen yöntem ve yol kalıplı yönlendirme (`"GET /charts/{id}"`) bu projeye yetiyor, ek bir web çatısına gerek kalmadı. Bağımlılık sayısı az kaldı.
- **pgx + goose:** pgx belgenin önerdiği sürücü. Migrasyonlar goose ile yönetilir ve `embed` ile programın içine gömülür, böylece tek dosyalık imaj kendi şemasını taşır.
- **bcrypt:** Veritabanında parolanın kendisi değil özeti saklanır.
- **JWT (HS256, access + refresh):** Token türü içeride tutulur; refresh token ile access token yerine geçilemez. Süresiz token kabul edilmez.
- **Giriş hatası tek mesaj:** Yanlış e-posta ile yanlış parola aynı yanıtı verir, kayıtlı e-postalar sızmaz.
- **Rol kontrolü sunucu tarafında:** Her korumalı uç, `Authenticate` ve `RequireRoles` ara katmanlarından geçer. Yetkisiz erişim 401, yetkisi olmayan rol 403 alır.
- **Hata biçimi:** Tüm hatalar `{"error":{"code":"...","message":"..."}}` biçimindedir. Beklenmeyen hataların ayrıntısı log'a yazılır, istemciye gösterilmez.
- **Yapılandırma:** Tüm ayarlar ortam değişkenlerinden gelir, loglar JSON biçimindedir.

## Durum

### Çalışan ve doğrulanan

| Konu | Durum |
|---|---|
| Docker ile tek komut, healthcheck, `service_healthy` bağımlılığı | Tamam |
| Çok aşamalı imaj, root olmayan kullanıcı | Tamam |
| Otomatik migrasyon ve demo hesap seed'i (tekrar çalıştırmak güvenli) | Tamam |
| Kimlik doğrulama, JWT, RBAC (401/403) | Tamam |
| **T-1 SPC kural motoru** | Tamam, aşağıdaki bölüme bakın |

### Yapılmadı

Aşağıdakiler henüz yapılmadı. Süre kısıtı nedeniyle önceliği T-1'e verdim.

- Frontend (3000 portu henüz bir servis sunmuyor)
- Veri üretici (simülatör) servisi
- F-1, F-2, F-3, F-5, F-6, F-7 kapsam maddeleri (iş emri/BOM, üretim kaydı, kalite formu, NCR, OEE panosu, izlenebilirlik raporu). F-4 yalnızca API düzeyinde var.
- T-2 (OEE), T-3 (soy ağacı), T-4 (iş emri ve malzeme düşümü), T-5 (yüksek hacimli girdi)
- `Idempotency-Key` desteği, denetim kaydı tablosu, keyset sayfalama
- OpenAPI dokümanı, `/metrics`, k6 yük testi, kaos betiği
- GitHub Actions, `make verify`, `TESTING.md`, `DECISIONS.md`, `requests.http`
- Refresh token üretiliyor ama yenileme ucu (`/auth/refresh`) yok

## T-1: SPC kural motoru

**Kontrol limitleri:** Bireysel (I-MR) ve X̄-R grafikleri desteklenir. Limitler ilk 25 noktadan otomatik hesaplanır ve sürüm 1 olarak saklanır. `POST /charts/{id}/limits/recompute` son 25 noktadan yeni bir sürüm üretir, eski sürümler silinmez. Her alarm, hangi limit sürümüne göre üretildiğini saklar.

**Kurallar:**

| Kural | Anlamı |
|---|---|
| `beyond_3_sigma` | Tek nokta merkezden 3 sigmadan uzakta |
| `same_side_8` | 8 ardışık nokta merkezin aynı tarafında |
| `trend_6` | 6 ardışık nokta sürekli artıyor ya da azalıyor |
| `two_of_three_beyond_2_sigma` | 3 noktadan 2'si aynı tarafta, 2 sigmadan uzakta |
| `four_of_five_beyond_1_sigma` | 5 noktadan 4'ü aynı tarafta, 1 sigmadan uzakta |

**Tekrar alarm üretmeme:** Desen kuralları, ihlal başladığında bir kez alarm verir; ihlal sürdüğü sürece yenisini üretmez. İki katmanlı korunur: uygulama katmanında dedektör, veritabanında `UNIQUE (chart_id, point_seq, rule)` kısıtı.

**Eşzamanlılık:** Ölçüm eklerken grafik satırı `SELECT ... FOR UPDATE` ile kilitlenir. Nokta, limit ve alarm aynı işlemde yazılır, hata olursa hepsi geri alınır.

**Bilinen sınırlamalar:**
- X̄-R grafiğinde kurallar yalnızca X̄ serisine uygulanır, R grafiğinin alarmı yoktur.
- "6 ardışık artış" kuralı, standart Nelson 3. kuralı gibi uygulanmıştır (6 nokta sürekli artış ya da azalış).
- İlk 25 ölçümün hepsi aynıysa sigma sıfır olur ve limit hesaplanamaz; 25. ölçüm reddedilir.
- Eşzamanlılık davranışı sahte depoda ve tasarımla doğrulandı, gerçek PostgreSQL ile yarış testi henüz yazılmadı.

**Kanıt komutları:**

```powershell
# Birim ve servis testleri
cd backend
go test ./internal/spc/ -v

# Çalışan sistem üzerinde uçtan uca doğrulama (önce: docker compose up --build -d)
cd ..
powershell -ExecutionPolicy Bypass -File .\scripts\demo-spc.ps1
```

Uçtan uca betiğin çıktısı:

```
PASS  request without token is rejected (401)
PASS  operator cannot create a chart (403)
PASS  quality engineer creates a chart (201)
PASS  operator cannot recompute limits (403)
PASS  25 baseline measurements are accepted
PASS  limits are computed on the 25th point (version 1)
PASS  UCL matches the hand calculation (about 15.279)
PASS  outlier 16 raises exactly one alarm
PASS  alarm is beyond_3_sigma on point 26
PASS  12 consecutive points above center raise only one alarm
PASS  chart detail shows both stored alarms
PASS  quality engineer creates limit version 2

ALL CHECKS PASSED
```

## Testler

Backend testleri yerelde şöyle çalışır (Go 1.22 veya üstü):

```powershell
cd backend
go test ./...
```

Şu an yaklaşık 100 test fonksiyonu var. Testlerin beklenen değerleri, kodun kendi çıktısından değil elle yapılan hesaplardan alınmıştır (örneğin kontrol limitleri).

## Doğrulama Soruları

Bu bölüm proje ilerledikçe tamamlanacak. Şu an yalnızca 5. soru cevaplanabilir durumda.

### 5. "SPC kural motoru" çözümünün doğru çalıştığını nasıl kanıtladım?

Üç katmanda:

1. **Elle hesap:** Limit formüllerinin beklenen değerlerini önce elle hesapladım, sonra testlere yazdım. Örneğin ortalaması 11 olan `10, 12, 11, 13, 9` serisi için UCL değeri 16,984 çıkıyor, test bunu doğruluyor.
2. **Kural testleri:** Her kural için hem alarm vermesi gereken hem vermemesi gereken durumlar var (tam limit üzerindeki nokta, karşı taraflara dağılmış noktalar, 12 ardışık noktada tek alarm, desen bozulup yeniden oluşunca ikinci alarm).
3. **Süreklilik testi:** `TestAddMeasurement_MatchesContinuousDetection`, 250 ölçümlük bir seriyi hem kesintisiz çalışan dedektöre hem de her ölçümde durumu yeniden kuran servise verir ve alarmların birebir aynı olduğunu doğrular. Böylece restart sonrası da aynı ihlal için tekrar alarm çıkmaz.

Komut ve çıktı: yukarıdaki "Kanıt komutları" bölümünde.