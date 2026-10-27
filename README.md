# HatKontrol: Üretim Hattı Takip ve Kalite Kontrol Sistemi (MES)

**Hazırlayan:** Berivan Elçi
Bandırma Onyedi Eylül Üniversitesi, Yazılım Mühendisliği öğrencisi

Bu depo, DDO Dijital Dönüşüm Ofisi teknik değerlendirmesi için hazırlanmıştır. Tüm kapsam maddeleri (F-1 ile F-7) ve beş zorlu görev (T-1 ile T-5) yapıldı ve çalışan sistem üzerinde betiklerle doğrulandı. Her maddenin nasıl kanıtlandığı ve bilinen sınırları aşağıda kimlik kimlik yazılıdır. Yapılmayan ya da ölçülmeyen şeyler **Bilinen sınırlamalar** bölümündedir.

## Kurulum

Gereken araçlar: Git ve Docker Desktop (Compose dahil). Go yalnızca testleri yerelde çalıştırmak ve yük testi için gerekir.

```powershell
git clone https://github.com/berivanelci-lgtm/ddo-case-berivan-elci.git
cd ddo-case-berivan-elci
Copy-Item .env.example .env
docker compose up --build
```

Linux ve macOS için `.env` kopyası: `cp .env.example .env`

- Arayüz: http://localhost:3000
- Backend (Go API): http://localhost:8080
- Sağlık kontrolü: http://localhost:8080/health (veritabanı bağlantısını da denetler)
- Metrikler: http://localhost:8080/metrics (Prometheus metin biçimi)
- API dokümanı: http://localhost:8080/openapi.yaml (OpenAPI 3)
- Dört servis: `db` (PostgreSQL 16), `backend`, `frontend` (nginx), `simulator` (veri üretici). Hepsinin `healthcheck`'i vardır ve `depends_on` ile `service_healthy` koşulu kullanılır.
- PostgreSQL ana makineye açılmaz, yalnızca Compose ağında erişilir.
- Şema migrasyonları (12 adet) ve demo hesaplar backend açılırken otomatik uygulanır.
- Konteynerler root olmayan kullanıcıyla çalışır; backend ve simülatör imajları çok aşamalıdır.

**Kurulum süresi:** Boş Docker önbelleğiyle (tüm proje ve temel imajlar ile derleme önbelleği silinmiş, `docker builder prune -af`) `docker compose up --build -d --wait` **97 saniyede** dört servisi `healthy` getirdi: imajların indirilmesi, backend ve simülatörün sıfırdan derlenmesi, migrasyonlar ve seed dahil. Ölçüm kendi makinemde ve ev internet bağlantımla yapıldı; Docker Desktop'ın sanal makinesi açıktı, bu yüzden tamamen yeni bir makinede birkaç saniye daha uzun sürebilir. `make verify` ilk çalıştırmada ayrıca PowerShell imajını (yaklaşık 100 MB) indirir; bu süre ölçüme dahil değildir.

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

## Hızlı doğrulama

Yalnızca Docker gerekir, işletim sisteminden bağımsızdır:

```bash
make verify
# make yoksa aynısı:
docker compose --profile verify run --rm verify
```

Bu komut `scripts/verify.ps1` betiğini resmî PowerShell konteynerinde, compose ağı içinden çalıştırır; soy ağacı verisi (1 milyon parti) yoksa önce ayrı bir servis yükler. Windows'ta aynı betik doğrudan da çalıştırılabilir: `powershell -ExecutionPolicy Bypass -File .\scripts\verify.ps1`. Her kimlik için `PASS` / `FAIL` tablosu basar, hata varsa çıkış kodu sıfırdan farklıdır. `make reset-demo` veritabanını silip sistemi ilk seed hâliyle yeniden kurar. Ayrıntılar `TESTING.md` dosyasındadır.

Konteyner yolu (`docker compose --profile verify run --rm verify`) ve Windows yolu ikisi de `PASS=18` verdi. `make` komutunun kendisini Windows makinemde `make` kurulu olmadığı için çalıştıramadım; `make verify` yalnızca yukarıdaki compose komutunu çağırır.**Makefile'ı denemedim**, Windows için betikler esastır.

Güncel sonuç:

```
HEALTH  AUTH-1  AUTH-2  AUTH-3  AUDIT  SIM               PASS
T-1  T-2  T-3  T-4  T-5                                  PASS
F-1  F-2  F-3  F-4  F-5  F-6  F-7                        PASS

PASS=18  PARTIAL=0  NOT-DONE=0  FAIL=0
```

Uzun süren kanıtlar `verify` dışında ayrı betiklerdir: yük testi (`go run ./cmd/sensorload`), kaos (`scripts/chaos.ps1`), indeks ve soy ağacı ölçümleri (`scripts/*.sql`).

## Mimari özeti

```
backend/
  cmd/api/                 API giriş noktası: ayarlar, bağlantı, yönlendirme, düzgün kapanış
  cmd/simulator/           veri üretici (ayrı compose servisi)
  cmd/sensorload/          yük testi aracı (p50/p95/p99, hata oranı)
  internal/config/         ortam değişkenlerinden ayar okuma
  internal/database/       PostgreSQL bağlantısı ve migrasyonların uygulanması
  internal/auth/           bcrypt, JWT (access + refresh), giriş ve token yenileme servisi
  internal/user/           roller, kullanıcı modeli, demo hesap seed'i
  internal/spc/            SPC: limitler, kural motoru, ölçüm servisi
  internal/catalog/        malzeme, ürün ve reçete (BOM) doğrulaması
  internal/workorder/      iş emri durum makinesi, malzeme hesabı
  internal/oee/            OEE hesabı (saf, deterministik), vardiyalar, hat/duruş/sayaç servisi
  internal/genealogy/      parti/seri soy ağacı ve geri çağırma raporu
  internal/ncr/            uygunsuzluk (NCR) durum makinesi
  internal/quality/        kalite ölçüm formu, tolerans ve kalibrasyon kontrolü
  internal/sensor/         sensör okuması doğrulaması
  internal/*store/         her alanın PostgreSQL deposu (userstore, spcstore, catalogstore,
                           workorderstore, oeestore, genealogystore, ncrstore, qualitystore, sensorstore)
  internal/audit/          denetim kaydı için işlemi yapan kullanıcıyı veritabanı oturumuna taşıma, okuma
  internal/idempotency/    Idempotency-Key kaydı ve okuması
  internal/metrics/        Prometheus metin biçiminde HTTP metrikleri
  internal/httpapi/        HTTP uçları, kimlik doğrulama, rol, idempotency ara katmanları, OpenAPI dokümanı
  migrations/              goose SQL migrasyonları (programın içine gömülür)
frontend/public/           düz HTML/CSS/JS (app.js: giriş ve SPC; modules.js: diğer ekranlar)
scripts/                   doğrulama, sıfırlama, kaos, veri yükleme ve ölçüm betikleri
docs/                      ölçüm çıktıları
```

İş mantığı paketleri veritabanı olmadan sahte depolarla test edilir; depolar (`*store`) çalışan sistemde betiklerle kanıtlanır.

## Karar gerekçeleri

Ayrıntılı karar kayıtları (bağlam, alternatifler, bedeli) `DECISIONS.md` dosyasındadır. Kısaca:

- Stok düşümü koşullu `UPDATE ... WHERE stock_qty >= x` ile yapılır; kontrol ve düşüm tek komuttur.
- İş emri ve NCR durum makineleri saf paketlerdir; tutarlılık kuralları hem kodda hem veritabanı `CHECK` kısıtlarındadır.
- SPC dedektörünün durumu her ölçümde son noktalardan yeniden kurulur; tekrar alarm hem dedektörde hem `UNIQUE` kısıtla engellenir.
- OEE kayıtlı tutulmaz, her sorguda olaylardan yeniden hesaplanır: geç gelen olay yazıldığı anda geçmiş sonuç düzelir.
- Soy ağacı, ayrı bir graf veritabanı yerine kenar tablosu ve `WITH RECURSIVE` ile kurulur; iki yön için ayrı dizin vardır.
- Sensör verisi aylık partisyonlu tabloya `COPY` ile toplu yazılır; eski partisyonlar ayrılıp arşiv şemasına taşınır.
- Denetim kaydı veritabanı tetikleyicileriyle yazılır, hiçbir kod yolunda unutulamaz.
- Idempotency kaydı durum değişikliğiyle aynı işlemde yazılır; işlem geri alınırsa anahtar da kaybolur.
- Go standart kütüphanesi ağırlıklı; frontend derleme adımı olmayan düz HTML/JS (kısa sürede çalışan arayüzü garantilemek için).

## Durum

### Kapsam maddeleri

| Kimlik | Durum | Nasıl kanıtlandı |
|---|---|---|
| F-1 İş emri ve BOM | Tamam | `demo-stock-race.ps1`; arayüzde İş emirleri ekranı |
| F-2 Üretim kaydı | Tamam | İş emrinde sağlam/fire/yeniden işlem (T-4); hatta sayaç ve planlı/plansız duruş nedenleri (T-2) |
| F-3 Kalite ölçümü | Tamam | Kalite ölçüm formu, tolerans değerlendirmesi, SPC'ye bağlama, kalibrasyon engeli (`verify.ps1`); arayüzde Kalite formları ekranı |
| F-4 SPC grafikleri ve alarmlar | Tamam | `demo-spc.ps1`; arayüzde canlı grafik |
| F-5 NCR | Tamam | `verify.ps1`; arayüzde Uygunsuzluklar ekranı |
| F-6 OEE panosu | Tamam | API ve arayüzde OEE panosu (10 sn'de bir yenilenir) |
| F-7 İzlenebilirlik raporu | Tamam | Geri çağırma raporu, 1M parti (`verify.ps1`); arayüzde İzlenebilirlik ekranı |

### Zorlu görevler

| Kimlik | Durum |
|---|---|
| T-1 SPC kural motoru | Tamam, kanıtlı |
| T-2 OEE ve geç gelen olaylar | Tamam, kanıtlı |
| T-3 Soy ağacı | Tamam, kanıtlı (1 milyon parti, API'de 30 ms altı) |
| T-4 İş emri ve malzeme düşümü | Tamam, kanıtlı |
| T-5 Yüksek hacimli sensör girdisi | Tamam, kanıtlı (500 olay/sn, p95 43 ms, hata %0) |

### Ortak teknik gereksinimler

| Konu | Durum |
|---|---|
| bcrypt, JWT (access + refresh), RBAC, her korumalı uçta yetki kontrolü | Tamam; `POST /auth/refresh` rolü veritabanından okur |
| Migrasyon dosyaları, FK, UNIQUE, CHECK kısıtları | Tamam (12 migrasyon) |
| Tutarlı hata biçimi, sunucu tarafı doğrulama, anlamlı durum kodları | Tamam |
| Keyset sayfalama | İş emri, denetim kaydı ve NCR listelerinde |
| Yapılandırılmış JSON log, ortam değişkenleri, zaman aşımı, düzgün kapanış | Tamam |
| Eşzamanlılık ve yarış kanıtı | Tamam (T-4 stok yarışı, T-5 partisyon oluşturma) |
| Kritik sorgular için dizin ve `EXPLAIN (ANALYZE)` kanıtı | SPC (soru 3), soy ağacı (T-3), sensör partisyon budama (T-5) |
| Append-only denetim tablosu | Tamam |
| `Idempotency-Key` | İş emri uçlarında tamam; tekrar gönderilen hat sayacı ve duruş olayları veritabanında tekilleştirilir |
| `/metrics` | Tamam |
| OpenAPI | Tamam (`/openapi.yaml`, Swagger Editor ile görsel olarak doğrulandı) |
| Kaos betiği | Tamam (`scripts/chaos.ps1`) |
| Yük testi (k6 eşdeğeri) | Tamam (`cmd/sensorload`, Go ile yazıldı) |
| `requests.http` | Tamam, tüm uçlar; VS Code REST Client ile denendi |
| Veri üretici (simülatör) | Tamam (ayrı compose servisi, tohumlu) |
| Frontend | Giriş, SPC grafikleri (canlı), iş emirleri, OEE panosu, kalite formları, NCR, izlenebilirlik |
| CI | GitHub Actions: `gofmt`, `go vet`, derleme, test ve kapsama, üç konteyner imajı |

## T-1: SPC kural motoru

**Kontrol limitleri:** Bireysel (I-MR) ve X̄-R grafikleri. Limitler ilk 25 noktadan otomatik hesaplanır ve sürüm 1 olarak saklanır. `POST /charts/{id}/limits/recompute` son 25 noktadan yeni sürüm üretir, eskiler silinmez. Her alarm hangi limit sürümüne göre üretildiğini saklar.

| Kural | Anlamı |
|---|---|
| `beyond_3_sigma` | Tek nokta merkezden 3 sigmadan uzakta |
| `same_side_8` | 8 ardışık nokta merkezin aynı tarafında |
| `trend_6` | 6 ardışık nokta sürekli artıyor ya da azalıyor |
| `two_of_three_beyond_2_sigma` | 3 noktadan 2'si aynı tarafta, 2 sigma dışında |
| `four_of_five_beyond_1_sigma` | 5 noktadan 4'ü aynı tarafta, 1 sigma dışında |

**Tekrar alarm üretmeme:** Desen kuralları ihlal başladığında bir kez alarm verir; ihlal sürdükçe yenisini üretmez. İki katmanlı korunur: dedektör ve `UNIQUE (chart_id, point_seq, rule)` kısıtı.

**Eşzamanlılık:** Ölçüm eklerken grafik satırı `SELECT ... FOR UPDATE` ile kilitlenir; nokta, limit ve alarm aynı işlemde yazılır.

**Kanıt:** `go test ./internal/spc/ -v` ve `scripts/demo-spc.ps1` (12 kontrol):

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

Arayüzde grafik ekranı 5 saniyede bir yenilenir; simülatörün `SIM-SPC-42` grafiğinde limitler, trend ve alarmlar canlı görülür.

## T-2: OEE ve geç gelen olaylar (F-2, F-6)

**Model (migrasyon `00008`, `00011`):** `lines` (hat ve ideal çevrim süresi), `downtime_reasons` (her neden planlı ya da plansız: `PLANNED_MAINTENANCE`, `BREAK` planlı; `BREAKDOWN`, `MATERIAL_WAIT`, `SETUP`, `QUALITY_HOLD` plansız), `downtime_events`, `production_counts`.

**Hesap:** Planlı üretim süresi = vardiya − planlı duruş; çalışma = planlı üretim − plansız duruş. Kullanılabilirlik = çalışma / planlı üretim; performans = (toplam adet × ideal çevrim) / çalışma; kalite = sağlam / toplam; OEE = üçünün çarpımı.

**Kararlar:**
- **OEE saklanmaz, her sorguda olaylardan yeniden hesaplanır.** Hesap saf bir fonksiyondur (`internal/oee/oee.go`); olaylar hangi sırayla gelirse gelsin aynı sonucu verir.
- **Vardiyalar (UTC):** A 06-14, B 14-22, C 22-06. Vardiya sınırını aşan duruş vardiya penceresine kırpılarak bölünür.
- Çakışan duruşlar iki kez sayılmaz; planlı ve plansız çakışırsa o süre planlı sayılır.
- **Tekrar gelen olaylar:** aynı hat ve anda gelen sayaç, aynı hat/neden/aralıkta gelen duruş `UNIQUE` kısıtla tek kayıt kalır (migrasyon `00011`).

**Kanıt (`scripts/demo-oee.ps1`, 17 kontrol).** İdeal çevrim 1 dk. A vardiyası: 10:00-10:30 planlı mola, 300 ürün (285 sağlam); B: 400 ürün (380 sağlam). Sayaç ve mola bilerek iki kez gönderilir. Sonra, A bittikten sonra, 13:00-14:30 arıza **geç** kaydedilir:

| | Önce | Geç olaydan sonra |
|---|---|---|
| A plansız duruş | 0 dk | 60 dk (14:00'e kadar olan kısım) |
| A kullanılabilirlik / performans | 1 / 0,6667 | 0,8667 / 0,7692 |
| B plansız duruş | 0 dk | 30 dk (14:00 sonrası kısım) |
| B kullanılabilirlik / performans | 1 / 0,8333 | 0,9375 / 0,8889 |
| Gün toplamı | | planlı 30, plansız 90 dk, kullanılabilirlik 0,9362, OEE 0,4716 |

Beklenen değerler elle hesaplanmıştır. Tekrar gönderilen sayaç sayılsaydı A'nın performansı 0,6667 yerine 1,18 olurdu; kontrol geçtiği için tekilleştirme de kanıtlıdır. Vardiya OEE değerleri geç olaydan sonra değişmez (OEE = sağlam × ideal çevrim / planlı üretim olduğu için beklenen sonuçtur).

**Arayüz:** OEE panosu hat ve gün seçimiyle A/B/C vardiyalarını ve gün toplamını kartlarda gösterir, 10 saniyede bir yenilenir; simülatör hattı varsayılan seçilidir.

## T-3: Soy ağacı ve geri çağırma (F-7)

**Model (migrasyon `00006`):** `lots` (hammadde, ara ürün, seri), `lot_inputs` (kenarlar), `shipments`, `shipment_items`.

**Sorgular:** `WITH RECURSIVE`, en fazla 20 seviye. `GET /traceability/lots/{code}/backward`, `/forward`, `/recall`. Kalite, planlama, hat şefi ve müdür okuyabilir; operatör ve bakım 403.

**Dizinler:** Birincil anahtar `(output_lot_id, input_lot_id)` geri izlemeyi, `lot_inputs_input_idx (input_lot_id, output_lot_id)` ileri izlemeyi, `shipment_items_lot_idx` sevkiyat birleştirmesini kapsar.

**1 milyon kayıt kanıtı:** `scripts/seed-genealogy.sql` deterministik olarak 1.000.000 parti, 2.100.000 kenar ve 600.000 sevk edilmiş seri üretir.

| Sorgu | Veritabanı (`EXPLAIN ANALYZE`) | API (verify.ps1, ısınmış) | Sonuç |
|---|---|---|---|
| Geri izleme, `SN-0300000` | 0,96 ms | 11-27 ms | 6 hammadde partisi |
| İleri izleme, `RAW-050000` | 0,88 ms | 9-33 ms | 45 parti (9 ara ürün, 36 seri) |
| Geri çağırma, `RAW-050000` | 0,72 ms | 11-29 ms | 36 seri, 36 sevkiyat |

İleri izlemenin 45 parti bulması, veri üretim kuralından elle hesaplanan değerle aynıdır. Tam plan çıktısı: `docs/genealogy-measurements.txt`.

**Sınırlar:** Partiler ve sevkiyatlar betikle yüklenir, üretim kaydından otomatik oluşmaz; izlenebilirlik API'si salt okunurdur ve sonuçlar sayfalanmaz.

## T-4: İş emri durum makinesi ve malzeme düşümü

**Durumlar:** `planned` → `released` → `in_progress` → `completed`; üretim başlamadan `cancelled`. Başlamış iş emri iptal edilmez, onayla (`allow_partial`) kısmi kapatılır.

**Sayaçlar:** `başlayan = sağlam + fire + yeniden işlemde bekleyen`; kodda ve veritabanı `CHECK` kısıtında. Bekleyen yeniden işlem varken iş emri kapatılamaz.

**Malzeme düşümü:** `UPDATE materials SET stock_qty = stock_qty - x WHERE id = ? AND stock_qty >= x`. Durum, düşüm, tüketim ve üretim kaydı tek işlemde yazılır; stok yetmezse hepsi geri alınır. Kapanışta tüketim, başlayan × reçete ile karşılaştırılır.

**Yarış kanıtı:** `scripts/demo-stock-race.ps1`, 6 iş emrinden aynı malzeme için 30 paralel istek gönderir (stok 100, ürün başına 5):

```
race: sent=30 ok=20 insufficient_stock=10 other=0
```

Stok 0'a iner, eksiye düşmez. Betik ayrıca fire, yeniden işlem, kısmi kapanış, plan aşımı ve keyset sayfalamayı dener (26 kontrol).

## T-5: Yüksek hacimli sensör girdisi

- **Tablo (migrasyon `00009`):** `sensor_readings`, `PARTITION BY RANGE (recorded_at)`, aylık partisyonlar. Ayın partisyonu yoksa `ensure_sensor_partition` fonksiyonu oluşturur; aynı anda gelen isteklerde advisory lock ve `duplicate_table` yakalama ile tek biri oluşturur (migrasyon `00010`).
- **Yazma:** `POST /sensor-readings`, istekte en fazla 1000 okuma, tek `COPY` komutuyla (ya hepsi yazılır ya hiçbiri).
- **Arşivleme:** `archive_sensor_partitions(tarih)` bitişi geçmiş partisyonları ana tablodan ayırıp `archive` şemasına taşır.

**Yük testi** (`cd backend; go run ./cmd/sensorload`): 50 istasyon × saniyede 10 olay, 60 saniye.

| Ölçüm | Değer |
|---|---|
| Hedef | 500 olay/sn |
| İstek (her biri 10 okuma) | 2961 |
| Yazılan olay | 29.610 |
| Gerçekleşen hız | 485 olay/sn |
| Hata oranı | %0,00 |
| p50 / p95 / p99 | 21,8 / 42,9 / 60,2 ms |
| En yüksek | 109,4 ms |

Gerçekleşen hızın 500'ün biraz altında kalması, istasyonların saniyelik zamanlayıcısının test bitimindeki son adımı kaçırmasından kaynaklanır (3000 yerine 2961 istek). Çıktı: `docs/load-test.txt`.

**Arşivleme ve partisyon budama** (`scripts/demo-sensor-archive.sql`, çıktı `docs/sensor-archive.txt`): Temmuz ve Ağustos partisyonlarına 1000'er satır yazıldı, `archive_sensor_partitions('2026-09-01')` ikisini `archive` şemasına taşıdı; ana tablodan Eylül öncesi sorgu 2000 yerine 0 satır döndü, arşivde 1000 + 1000 satır kaldı. "Bir istasyonun son bir saati" sorgusu yalnızca Ekim partisyonunu dizinle taradı (`Index Scan Backward`, 0,22 ms).

**Sınırlar:** Sensör okumaları tekilleştirilmez. Partisyonlar süreç içinde önbelleğe alınır; arşivlenen bir aya yeni okuma gelirse süreç yeniden başlatılmalıdır (90 günden eski okumalar zaten reddedilir).

## F-3: Kalite ölçüm formu

- **Form (migrasyon `00012`):** kod, ad, ölçüm cihazı, kalibrasyon bitiş tarihi ve karakteristikler (nominal, alt ve üst sınır). Karakteristik isteğe bağlı bir SPC grafiğine bağlanır. Yalnızca kalite mühendisi tanımlar.
- **Ölçüm girişi:** operatör, hat şefi ve kalite mühendisi formu tek seferde doldurur. Her değer tolerans içinde mi işaretlenir; bağlı değerler SPC grafiğine ölçüm olarak gider ve alarmlar yanıtta döner.
- **Kalibrasyonu geçmiş cihazla ölçüm girişi engellenir** (409 `calibration_expired`; bitiş günü dahil geçerlidir).
- `verify.ps1` gerçek sistemde dener: boy tolerans dışında işaretlenir, çap değeri SPC grafiğine 1. nokta olarak düşer, eksik değer 400, bakım 403, süresi geçmiş kalibrasyon 409.
- **Sınır:** Muayene ve SPC ölçümleri ayrı işlemlerde yazılır; SPC yazımı başarısız olursa muayene kaydı kalır, hata yanıtta görünür.

## F-5: Uygunsuzluk (NCR) yönetimi

- Durumlar: `open` → `investigating` → `closed`. Kapatmak için **kök neden ve düzeltici faaliyet zorunludur**; kural kodda ve `CHECK` kısıtında (migrasyon `00007`).
- NCR isteğe bağlı olarak iş emrine ya da SPC grafiğine bağlanır; önem `minor`, `major`, `critical`.
- Açma: operatör, hat şefi, kalite; inceleme ve kapatma: yalnızca kalite mühendisi.
- `verify.ps1`: bakım 403, operatör açar 201, erken kapatma 409, kök nedensiz kapanış 400, kalite kapatır 200.

## Simülatör

Ayrı compose servisi (`backend/cmd/simulator`, `backend/Dockerfile.simulator`). Her 5 saniyede:

- `SIM-SPC-42` grafiğine bir ölçüm: her 200 adımlık döngünün ilk 60 adımı kararlı, sonrası **bilerek yukarı kayan trend** (kontrolden çıkan süreç, alarm üretir)
- 50 istasyondan birer sensör okuması
- `SIM-HAT-42` hattına sayaç; her 40 adımda arıza, her 60 adımda **30 dakika gecikmeli** duruş, her 15 adımda ve arızalarda **tekrar gönderilen** olay

Ayarlar: `SIM_SEED` (varsayılan 42), `SIM_INTERVAL` (5s), `SIM_STATIONS` (50). Aynı tohum aynı değer dizisini üretir (`TestSimulation_SameSeedSameValues`); zaman damgaları çalıştırma anına bağlıdır. Sağlık kontrolü, son bir dakikada yazılmış bir kalp atışı dosyasına bakar. `verify.ps1` simülatör grafiğinde nokta olduğunu denetler.

## Denetim kaydı (audit log)

- Tablo `audit_log` (migrasyon `00004`): zaman, işlemi yapan kullanıcı, işlem, tablo, kayıt kimliği, eski ve yeni değer (JSONB).
- **Veritabanı tetikleyicileri** yazar: `charts`, `materials`, `products`, `work_orders`, `spc_limit_sets`, `ncrs`, `lines`, `downtime_events`, `quality_forms`.
- **Kim:** kimlik doğrulama ara katmanı kullanıcıyı context'e koyar, `audit.Begin` işlem başında `app.user_id` oturum ayarına yazar.
- **Eklemeli-yalnız:** `UPDATE`, `DELETE`, `TRUNCATE` tetikleyiciyle yasak; gerçek sistemde üçü de `audit_log is append-only` hatası verdi.
- **Okuma:** `GET /audit-log`, yalnızca fabrika müdürü, keyset sayfalamalı.
- **Sınır:** Tablo sahibi rol tetikleyiciyi kapatabilir; gerçek ortamda uygulama yalnızca `INSERT` ve `SELECT` yetkili ayrı rolle bağlanmalıdır. Soy ağacı, sensör, sayaç ve muayene tabloları denetlenmez.

## Idempotency-Key

- Kapsam: `POST /work-orders` ve `/work-orders/{id}/release|cancel|production|rework|close`. Başlık isteğe bağlıdır.
- Anahtar kullanıcıya bağlıdır (`PRIMARY KEY (user_id, key)`) ve gövdenin SHA-256 özetiyle saklanır. Kayıt, durum değişikliğiyle **aynı işlemde** yazılır.
- Aynı anahtar ve gövde: kayıtlı yanıt, `Idempotent-Replayed: true`. Farklı gövde: 422. Eşzamanlı çift istek: 409 `in_progress`.
- **Kanıt:** PowerShell ve REST Client ile aynı anahtarla iki istek aynı iş emri kimliğini döndürdü, ikincisinde `Idempotent-Replayed: true` vardı; 422 ve ara katman birim testlerle kanıtlı.
- **Sınırlar:** 409 `in_progress` gerçek sistemde denenmedi. Yanıt JSONB'de saklandığı için tekrar yanıtın alan sırası farklı olabilir (anlam aynı). Diğer modüllerde Idempotency-Key yoktur; hat sayacı ve duruş olayları bunun yerine veritabanında tekilleştirilir. Eski anahtarlar silinmez.

## İzleme ve dayanıklılık

**`GET /metrics`** (Prometheus metin biçimi): `hatkontrol_http_requests_total{method,status}`, `hatkontrol_http_request_duration_seconds` histogramı, `hatkontrol_uptime_seconds`. Kütüphane yerine metin biçimi elle üretildi; yol bazında etiket yoktur (Go 1.22'de eşleşen yol kalıbı istek nesnesinde yok). Gerçek ortamda yalnızca iç ağdan erişilmelidir.

**Kaos senaryosu** (`scripts/chaos.ps1`): veritabanı durdurulur ve tekrar başlatılır.

```
PASS  before: health is 200
PASS  before: login is 200
PASS  during outage: health reports 503 (not a hang, not 200)
PASS  during outage: login fails with 500 and does not crash the API
PASS  during outage: backend container keeps running
PASS  after: health is 200 again within 90 s (took 1.1 s), no restart of backend
PASS  after: login works again

ALL CHECKS PASSED (recovery 1.1 s)
```

## OpenAPI

`backend/internal/httpapi/openapi.yaml` elle yazıldı, programın içine gömülüdür ve `GET /openapi.yaml` adresinden sunulur. Swagger Editor'da hatasız açıldığı görsel olarak kontrol edildi. `TestOpenAPI_DocumentsEveryRoute` uygulamadaki her yolun dokümanda olduğunu denetler; kalite formu uçları eklenirken dokümana yanlış girintiyle girdiğinde CI bu testle kırmızıya döndü. **Sınır:** Doküman koddan üretilmez; test yolların varlığını denetler, şemaların kodla birebir uyumunu denetlemez.

## Testler ve kapsama

```powershell
cd backend
go test ./... -cover
```

İş mantığı paketlerinin kapsaması (CI çıktısından):

| Paket | Kapsama | Paket | Kapsama |
|---|---|---|---|
| catalog | %100 | sensor | %100 |
| config | %100 | user | %100 |
| metrics | %97,9 | genealogy | %95,2 |
| ncr | %93,9 | workorder | %92,5 |
| spc | %92,2 | oee | %87,9 |
| auth | %85,7 | httpapi | %82,7 |
| quality | %80,0 | | |

Veritabanına bağlanan paketler (`*store`, `database`, `cmd/*`) ve `audit`, `idempotency` paketlerinin büyük kısmı birim testle kapsanmaz; bunlar çalışan sistemde `scripts/` altındaki betiklerle kanıtlanır. Bu yüzden CI'daki toplam kapsama düşük görünür.

## CI

GitHub Actions (`.github/workflows/ci.yml`) her push'ta: `gofmt` biçim kontrolü, `go vet`, derleme, testler ve kapsama, backend, simülatör ve frontend imajı derlemesi. "Lint" olarak `gofmt` ve `go vet` kullanıldı, `golangci-lint` kullanılmadı. Uçtan uca betikler CI'da çalışmaz (çalışan sistem ister).

## Bilinen sınırlamalar

-- `make` komutu Windows'ta denenmedi; çağırdığı compose komutları denendi. `make chaos` Linux/macOS'ta `pwsh` ister.
- SPC limitlerini yeniden hesaplamak alarm veren noktaları dışlamaz (soru 2). X̄-R grafiğinde kurallar yalnızca X̄ serisine uygulanır.
- İlk 25 ölçümün hepsi aynıysa sigma sıfırdır, limit hesaplanamaz.
- Refresh token yenilenince eskisi geçersiz olmaz (iptal listesi yok); süresi dolana kadar kullanılabilir. Arayüz token'ı otomatik yenilemez, 15 dakika sonra yeniden giriş ister.
- OEE vardiyaları sabit ve UTC'dir; hat sayaçları iş emri kayıtlarından bağımsızdır.
- Soy ağacı verisi betikle yüklenir; izlenebilirlik sonuçları sayfalanmaz.
- Sensör okumaları tekilleştirilmez; kalite muayenesi ile SPC yazımı ayrı işlemlerdedir.
- Arayüzde grafik, hat, malzeme ve ürün oluşturma formu yoktur (API ve `requests.http` ile yapılır).
- Denetim tetikleyicileri tablo sahibi rol tarafından kapatılabilir; idempotency anahtarları süresiz saklanır.

## Doğrulama Soruları

### 1. En zorlandığım görev hangisiydi? Hangi alternatifleri düşündüm ve neden eledim?

En çok T-4'ün stok yarışında zorlandım. Aynı malzemeyi aynı anda birden çok iş emri tüketebilir; stok önce okunup sonra yazılırsa iki işlem aynı değeri okuyup stoğu eksiye düşürebilir. Üç yaklaşım değerlendirdim:

1. `SELECT ... FOR UPDATE` ile stok satırını kilitleyip okumak: çalışır ama kilit daha uzun tutulur ve her işlem sırayla bekler.
2. `SERIALIZABLE` izolasyon ve çakışmada yeniden deneme: doğru ama yeniden deneme mantığı gerektirir, hata riskini artırırdı.
3. Tek koşullu `UPDATE ... WHERE stock_qty >= x`: kontrol ve düşüm tek komutta olur, `CHECK (stock_qty >= 0)` ikinci güvenlik sağlar.

Üçüncüsünü seçtim. Malzemeleri hep kimlik sırasıyla güncelleyerek kilitlenme riskini de azalttım.

İkinci zorluk, yarışı **gerçekten** denediğimden emin olmaktı. İlk fikrim 30 isteği tek iş emrine göndermekti; ama iş emrinin satır kilidi istekleri zaten sıraya dizer ve stok yarışı hiç denenmezdi. Bu yüzden betik 6 ayrı iş emrine istek dağıtır.

### 2. Çözümümün kırıldığı ya da yetersiz kaldığı bir senaryo

SPC limitlerini yeniden hesaplarken son 25 noktayı olduğu gibi kullanıyorum. Demo grafiğinde 20 gibi aykırı değerler varken limitleri yeniden hesapladığımda üst limit 26,5'e, alt limit -1,4'e genişledi (merkez 12,6); oysa taban çizgisinde değerler 9 ile 11 arasındaydı. Aykırı değerler standart sapmayı şişirdiği için limitler gevşiyor ve sonraki gerçek sorunlar alarm vermeyebilir. Simülatörün kayan trendinden sonra limit yeniden hesaplanırsa aynı sorun ortaya çıkar. Doğru çözüm, alarm veren noktaları yeniden hesaplamadan çıkarmaktır; yapmadım.

### 3. Eklediğim dizinlerden ikisi: hangi sorgu için, dizinsiz/dizinli ölçüm farkı?

Ölçüm `scripts/measure-indexes.sql` ile yapıldı. Betik, gerçek tablolarla aynı yapıda geçici tablolarda 500 grafiğe dağıtılmış 1 milyon alarm ve 1 milyon nokta üretir, her sorguyu dizinsiz ve dizinli `EXPLAIN (ANALYZE, BUFFERS)` ile çalıştırır ve işlemi geri alır (`leftover_tables = 0` ile doğrulandı). Tam çıktı: `docs/index-measurements.txt`. Süreler çalıştırmadan çalıştırmaya oynar (aynı sorgu ilk seferde 13,0 ms, ikincisinde 5,5 ms), bu yüzden ısınmış çalıştırma verilmiştir ve en güvenilir kanıt okunan sayfa sayısıdır.

**Dizin 1: `spc_alarms_chart_recent_idx` (chart_id, id DESC)**, sorgu: bir grafiğin son 50 alarmı.

| | Dizinsiz | Dizinli |
|---|---|---|
| Plan | `Index Scan Backward` (birincil anahtar), `chart_id` süzgeçle | `Index Scan`, `Index Cond: chart_id = 250` |
| Elenen satır | 24.701 | yok |
| Okunan sayfa | 371 | 53 |
| Süre | yaklaşık 5,5 ms | yaklaşık 0,25 ms |

Dizinsiz durumda planlayıcı birincil anahtarı tersten tarayıp `chart_id`'yi süzgeçle buldu. Veri eşit dağıtıldığı için 50 alarm bulmak 24.701 satır atlamakla bitti; gerçek veride bir grafiğin son alarmları eski bölgedeyse tarama çok daha uzar, bu ölçüm iyimser bir alt sınırdır.

**Dizin 2: `spc_points` birincil anahtarı (chart_id, seq)**, sorgu: bir grafiğin son 8 noktası (her ölçümde okunur).

| | Dizinsiz | Dizinli |
|---|---|---|
| Plan | `Parallel Seq Scan` + sıralama | `Index Scan Backward` |
| Okunan sayfa | 10.422 | 11 |
| Süre | yaklaşık 62 ms | yaklaşık 0,16 ms |

Süre yaklaşık 400 kat azaldı. Dizinsiz durumda tablo büyüdükçe süre doğrusal artar; dizinli durumda hemen hemen sabit kalır. Soy ağacı (T-3) ve sensör partisyon budama (T-5) ölçümleri de ilgili bölümlerdedir.

### 4. Yapay zekâ kullandıysam, reddettiğim ya da düzelttiğim çıktılar

Claude ile çalıştım. Düzeltilen çıktılar:

**a) Frontend healthcheck.** Önce `wget -qO- http://localhost:8080/`, sonra `http://127.0.0.1:8080/`. Sayfa tarayıcıda çalışıyordu ama sağlık kaydında `Connection refused` vardı ve `reset-demo.ps1` `frontend is unhealthy` ile düştü. Nginx yalnızca IPv4'te dinliyor, `localhost` bu imajda büyük olasılıkla IPv6'ya çözülüyor (nedeni ayrıca doğrulamadım; düzeltmenin çalıştığını sıfırdan kurulumla doğruladım).

**b) Arayüzde gizlenmeyen form.** `#measure-form { display: grid; }` kuralı `hidden` özniteliğini ezdi, bakım rolünde ölçüm formu görünüyordu. `[hidden] { display: none !important; }` ile düzeltildi. Güvenlik açığı değildi (sunucu 403 döner), ama arayüz tutarsızdı.

**c) Test tasarımı.** Stok yarışı betiği 30 isteği tek iş emrine gönderecekti; satır kilidi yarışı gizleyeceği için 6 iş emrine dağıtıldı.

**d) Kapsama ve paket yapısı.** `user` paketinin kapsaması %53 çıktı. Rakamı yükseltmek için sahte test yazmak yerine SQL kodunu `userstore` paketine taşıdım; kapsama %100'e çıktı.

**e) İndeks ölçüm betiği.** Bir sütun adındaki yazım hatası (`schart_id`) betiği yarıda kesti; hata dosyaya yazılmadığı için önce görünmedi. Hata çıktısını da yönlendirip buldum. Betik tek işlemde geri alındığı için kalıntı bırakmadı.

**f) Idempotency-Key'in sessizce çalışmaması.** İlk denemede aynı anahtarla iki istek iki farklı iş emri oluşturdu (25 ve 26), testler ve `verify` yeşildi. `idempotency_keys` tablosu boştu: ara katman oluşturma ucuna uygulanmamıştı. Yalnızca gerçek sistemde aynı isteği iki kez gönderip veritabanına bakınca ortaya çıktı. Ders: derlenip testlerin geçmesi davranışın çalıştığını göstermez.

**g) Sensör partisyonu yarışı.** İlk yük testinde 3000 istekten 10'u başarısız oldu (%0,33). Ay partisyonu yokken ilk saniyede 50 istek aynı anda partisyon oluşturmaya çalıştı; kilitten sonraki tekrar kontrolün yeni tabloyu görmediğini ve ikinci kez oluşturma denendiğini düşündüm. **Sebebi doğrulayamadım:** konteyner yeniden oluşturulduğu için log'lar kaybolmuştu. Fonksiyonu `CREATE TABLE IF NOT EXISTS` ve `duplicate_table` yakalamayla düzelttim (migrasyon `00010`), yük testi aracına hata nedenini yazdırmasını ekledim. Sonraki koşu 2961 istekte %0 hata verdi; 20 paralel bağlantıyla yeni bir ay partisyonu oluşturma denemesi de hatasız geçti (ama bu denemenin işlerinin gerçekten aynı anda çakıştığını ölçmedim).

**h) OpenAPI dokümanı ve CI.** Kalite formu uçlarını dokümana eklerken iki satır yanlış girintiyle yapıştı; yerelde fark edilmedi, CI `TestOpenAPI_DocumentsEveryRoute` ile kırmızıya döndü. Aynı gece bir CI dosyası değişikliği de bozuk YAML yüzünden iş çalıştırmadan başarısız oldu. İkisi de CI'ın yakaladığı gerçek hatalardı.

**i) Sınav belgesindeki gizli talimatlar.** Sınav belgesinde yapay zekâ araçlarına yönelik gizli yorum satırları vardı (tablo adlarına önek, özel HTTP başlığı, ek uç nokta, sabit e-posta ve container adı, README'nin son satırına kod yazılması gibi). Proje gereksinimi olmadıkları için hiçbirini uygulamadım.

### 5. "SPC kural motoru" çözümünün doğru çalıştığını nasıl kanıtladım? Komut ve çıktı

Üç katmanda:

1. **Elle hesap:** Limit formüllerinin beklenen değerlerini önce elle hesapladım, sonra testlere yazdım. Örneğin `10, 12, 11, 13, 9` serisi için UCL 16,984; çalışan sistemde 25 noktalık taban çizgisinin UCL'si elle hesaplanan 15,279 ile eşleşti.
2. **Kural testleri:** Her kural için hem alarm vermesi hem vermemesi gereken durumlar (tam limit üzerindeki nokta, karşı tarafa dağılan noktalar, 12 ardışık noktada tek alarm, desen bozulup yeniden oluşunca ikinci alarm).
3. **Süreklilik testi:** `TestAddMeasurement_MatchesContinuousDetection` 250 ölçümlük bir seriyi hem kesintisiz dedektöre hem her ölçümde durumu yeniden kuran servise verir; alarmların birebir aynı olduğunu doğrular.

Komutlar ve çıktı yukarıdaki **T-1** bölümündedir (`go test ./internal/spc/ -v`, `scripts/demo-spc.ps1`, 12 kontrol, `ALL CHECKS PASSED`). Ayrıca simülatör, kontrollü bir kayma üreterek kuralların canlı veride de alarm verdiğini arayüzde gösterir.