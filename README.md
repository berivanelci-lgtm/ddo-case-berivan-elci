# HatKontrol: Üretim Hattı Takip ve Kalite Kontrol Sistemi (MES)

**Hazırlayan:** Berivan Elçi
Bandırma Onyedi Eylül Üniversitesi, Yazılım Mühendisliği öğrencisi

Bu depo, DDO Dijital Dönüşüm Ofisi teknik değerlendirmesi için hazırlanmıştır.
**Proje tamamlanmış değildir.** Süre kısıtı nedeniyle iki zorlu görevi (T-1 SPC kural motoru, T-4 iş emri ve malzeme düşümü) derinlemesine yaptım ve gerçek sistemde kanıtladım; ortak teknik gereksinimlerden denetim kaydı ve Idempotency-Key'i ekledim. Diğer görevler yapılmadı. Neyin çalıştığı ve neyin yapılmadığı aşağıdaki **Durum** bölümünde, kimlik kimlik, açıkça yazılıdır.

## Kurulum

Gereken araçlar: Git ve Docker Desktop (Compose dahil). Go yalnızca testleri yerelde çalıştırmak için gerekir.

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
- PostgreSQL 16 ana makineye açılmaz, yalnızca Compose ağında erişilir.
- Şema migrasyonları (5 adet) ve demo hesaplar backend açılırken otomatik uygulanır.
- Üç servisin de `healthcheck`'i vardır; `depends_on` ile `service_healthy` koşulu kullanılır (db, backend, frontend sırasıyla).
- Konteynerler root olmayan kullanıcıyla çalışır; backend imajı çok aşamalıdır.

**Kurulum süresi:** Önbellekli bir makinede `scripts/reset-demo.ps1` ile sıfırdan kurulum (veritabanı silinir, yeniden oluşturulur) yaklaşık 18 saniyede tamamlandı. **Boş Docker önbelleğiyle 5 dakika sınırını ölçmedim.**

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

Sistem ayaktayken (Windows PowerShell):

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\verify.ps1
```

Her kimlik için `PASS` / `PARTIAL` / `NOT-DONE` / `FAIL` tablosu basar; yalnızca `FAIL` varsa çıkış kodu sıfırdan farklıdır. `scripts/reset-demo.ps1` verileri ilk seed haline döndürür (veritabanını siler). Ayrıntılar `TESTING.md` dosyasındadır. Makefile (`make verify`, `make reset-demo`) aynı betikleri çağırır; **Makefile'ı denemedim**, Windows için betikler esastır.

Güncel sonuç:

```
HEALTH   PASS      AUTH-1   PASS      AUTH-2   PASS      AUTH-3   PASS
T-1      PASS      T-4      PASS      F-1      PASS      F-4      PASS
AUDIT    PASS
F-2      PARTIAL   F-3      PARTIAL
F-5      NOT-DONE  F-6      NOT-DONE  F-7      NOT-DONE
T-2      NOT-DONE  T-3      NOT-DONE  T-5      NOT-DONE

PASS=9  PARTIAL=2  NOT-DONE=6  FAIL=0
```

## Mimari özeti

```
backend/
  cmd/api/                 giriş noktası: ayarlar, bağlantı, yönlendirme, düzgün kapanış
  internal/config/         ortam değişkenlerinden ayar okuma
  internal/database/       PostgreSQL bağlantısı ve migrasyonların uygulanması
  internal/auth/           bcrypt parola özeti, JWT (access + refresh), giriş servisi
  internal/user/           roller, kullanıcı modeli, demo hesap seed'i (saf iş mantığı)
  internal/userstore/      kullanıcıların PostgreSQL deposu
  internal/spc/            SPC saf mantığı: limitler, kural motoru, ölçüm servisi
  internal/spcstore/       SPC'nin PostgreSQL deposu
  internal/catalog/        malzeme, ürün ve reçete (BOM) doğrulaması
  internal/catalogstore/   katalogun PostgreSQL deposu
  internal/workorder/      iş emri durum makinesi, malzeme hesabı, servis
  internal/workorderstore/ iş emirlerinin PostgreSQL deposu
  internal/audit/          denetim kaydı için işlemi yapan kullanıcıyı veritabanı oturumuna taşıma, okuma
  internal/idempotency/    Idempotency-Key kaydı ve okuması
  internal/httpapi/        HTTP uçları, kimlik doğrulama, rol kontrolü ve idempotency ara katmanları
  migrations/              goose SQL migrasyonları (programın içine gömülür)
frontend/                  düz HTML/CSS/JS, root olmayan nginx ile sunulur
scripts/                   uçtan uca doğrulama, sıfırlama ve indeks ölçüm betikleri
docs/                      ölçüm çıktıları
```

Katmanlar arası bağımlılık arayüzlerle kurulmuştur; iş mantığı (`auth`, `spc`, `workorder`, `user`) veritabanı olmadan sahte depolarla test edilebilir.

## Karar gerekçeleri

Ayrıntılı karar kayıtları (bağlam, alternatifler, bedeli) `DECISIONS.md` dosyasındadır (8 karar). Kısaca:

- Stok düşümü koşullu `UPDATE ... WHERE stock_qty >= x` ile yapılır (K-1).
- İş emri durum makinesi saf pakettir; sayaç tutarlılığı hem kodda hem veritabanı `CHECK` kısıtında korunur (K-2).
- SPC dedektörünün durumu her ölçümde son noktalardan yeniden kurulur (K-3); tekrar alarm iki katmanda engellenir (K-4).
- Yetki kontrolü sunucuda ara katmanlarla yapılır (K-5).
- Go standart kütüphanesi (K-6), düz HTML/JS frontend (K-7), test stratejisi (K-8).
- Frontend seçimi: derleme adımı olmayan düz HTML/CSS/JS. Gerekçe: kısa süre içinde çalışan bir arayüzü garantilemek, çerçeve kurulumu ve derleme hatası riskini almamak.
- Denetim kaydı veritabanı tetikleyicileriyle yazılır: hiçbir kod yolunda unutulamaz, eski ve yeni değer eksiksiz gelir. "Kim" bilgisi token'dan alınıp işlem başında `app.user_id` oturum ayarına yazılır.
- Idempotency kaydı, durum değişikliğiyle aynı veritabanı işleminde yazılır: işlem geri alınırsa anahtar da kaybolur, yarım kalmış bir işlem "yapıldı" sayılmaz.

## Durum

### Kapsam maddeleri

| Kimlik | Durum | Not |
|---|---|---|
| F-1 İş emri ve BOM | Tamam (API) | Arayüz ekranı yok |
| F-2 Üretim kaydı | Kısmen | Sayaç, fire, yeniden işlem var; **duruş nedenleri yok** |
| F-3 Kalite ölçümü | Kısmen | Ölçüm girişi SPC grafiği üzerinden; ayrı bir kalite formu tanımı yok |
| F-4 SPC grafikleri ve alarmlar | Tamam | API ve arayüz |
| F-5 NCR | Yapılmadı | |
| F-6 OEE panosu | Yapılmadı | |
| F-7 İzlenebilirlik raporu | Yapılmadı | |

### Zorlu görevler

| Kimlik | Durum |
|---|---|
| T-1 SPC kural motoru | Tamam, kanıtlı |
| T-2 OEE ve geç gelen olaylar | Yapılmadı |
| T-3 Soy ağacı | Yapılmadı |
| T-4 İş emri ve malzeme düşümü | Tamam, kanıtlı |
| T-5 Yüksek hacimli sensör girdisi | Yapılmadı |

### Ortak teknik gereksinimler

| Konu | Durum |
|---|---|
| bcrypt, JWT (access + refresh), RBAC, her korumalı uçta yetki kontrolü | Tamam (refresh token üretiliyor ama **yenileme ucu yok**) |
| Migrasyon dosyaları, FK, UNIQUE, CHECK kısıtları | Tamam |
| Tutarlı hata biçimi, sunucu tarafı doğrulama, anlamlı durum kodları | Tamam |
| Keyset sayfalama | İş emri listesi ve denetim kaydı listesinde |
| Yapılandırılmış JSON log, ortam değişkenleri, zaman aşımı, düzgün kapanış | Tamam |
| Eşzamanlılık (transaction, kilit, koşullu UPDATE) ve yarış kanıtı | Tamam (T-4) |
| Kritik sorgular için dizin ve `EXPLAIN (ANALYZE)` kanıtı | İki dizin için ölçüldü (Doğrulama Soruları, 3. soru) |
| Append-only denetim tablosu | Tamam (aşağıdaki bölüme bakın) |
| `Idempotency-Key` | İş emri uçlarında tamam (aşağıdaki bölüme bakın); **SPC ölçüm ucunda yok** |
| OpenAPI, `/metrics` | **Yapılmadı** |
| k6 yük testi, kaos betiği | **Yapılmadı** |
| `requests.http` (VS Code REST Client) | Eklendi, **ben çalıştırıp denemedim** |
| Veri üretici (simülatör) servisi | **Yapılmadı** |
| Frontend | Giriş, grafik listesi, grafik detayı (ölçüm girişi, alarmlar). İş emri, katalog ve diğer modüllerin ekranı yok |

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

**Kanıt komutları:**

```powershell
cd backend
go test ./internal/spc/ -v
cd ..
powershell -ExecutionPolicy Bypass -File .\scripts\demo-spc.ps1
```

`demo-spc.ps1` çıktısı (12 kontrol):

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

## T-4: İş emri durum makinesi ve malzeme düşümü

**Durumlar:** `planned` → `released` → `in_progress` → `completed`; üretim başlamadan `cancelled` olabilir. Başlamış bir iş emri iptal edilmez (malzeme düşmüştür), onayla (`allow_partial`) kısmi olarak kapatılır.

**Sayaçlar:** `başlayan = sağlam + fire + yeniden işlemde bekleyen`. Bu kural kodda her işlemden sonra denetlenir ve veritabanında `CHECK` kısıtıdır. Fire plana sayılmaz; sağlam ve bekleyen toplamı plandan fazla olamaz. Bekleyen yeniden işlem varken iş emri kapatılamaz.

**Malzeme düşümü:** Başlayan her ürün için reçeteye göre `UPDATE materials SET stock_qty = stock_qty - x WHERE id = ? AND stock_qty >= x` ile düşülür. Durum, düşüm, tüketim kaydı ve üretim kaydı tek veritabanı işleminde yazılır; stok yetmezse hepsi geri alınır. Kapanışta tüketilen malzeme, başlayan adet × reçete miktarı ile karşılaştırılır.

**Yarış kanıtı:** `scripts/demo-stock-race.ps1`, 6 farklı iş emrinden aynı malzeme için 30 paralel üretim isteği gönderir (stok 100, ürün başına 5 birim). Beklenen ve gerçekleşen sonuç:

```
race: sent=30 ok=20 insufficient_stock=10 other=0
```

Stok 0'a iner ve eksiye düşmez; kapanışta malzeme denetimi geçer. Betik ayrıca fire, yeniden işlem döngüsü, kısmi kapanış, plan aşımı ve keyset sayfalamayı dener (toplam 26 kontrol, hepsi `PASS`). Denetim tetikleyicileri ve idempotency eklendikten sonra da yeniden çalıştırıldı ve geçti.

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\demo-stock-race.ps1
```

**Sınırlamalar:** Miktarlar tam sayıdır (kesirli birim yok). Yarış kanıtı çalışan sistemde betikle alınır, CI'da otomatik çalışmaz. `workorder` paketindeki goroutine'li test sahte depo kullanır ve yalnızca servis mantığını doğrular; gerçek PostgreSQL kanıtı betiktir.

## Denetim kaydı (audit log)

- Tablo: `audit_log` (migrasyon `00004`). Alanlar: zaman, işlemi yapan kullanıcı, işlem (`insert`/`update`/`delete`), tablo adı, kayıt kimliği, eski değer, yeni değer (JSONB).
- Kayıtları **veritabanı tetikleyicileri** yazar: `charts`, `materials`, `products`, `work_orders`, `spc_limit_sets`. Böylece stok düşümü dahil her değişiklik, hangi kod yolundan gelirse gelsin kaydedilir.
- **Kim:** Kimlik doğrulama ara katmanı kullanıcıyı context'e koyar; depolar işlemi `audit.Begin` ile başlatır ve kullanıcı kimliğini `app.user_id` oturum ayarına yazar. API dışından yapılan değişikliklerde `user_id` boş kalır.
- **Eklemeli-yalnız:** `UPDATE`, `DELETE` ve `TRUNCATE` tetikleyiciyle yasaktır. Gerçek sistemde denendi, üçü de `audit_log is append-only` hatası verdi.
- **Okuma:** `GET /audit-log?entity_type=...&limit=...&before=...`, yalnızca fabrika müdürü (diğer roller 403). Keyset sayfalamalı.
- **Sınır:** Tablo sahibi olan veritabanı rolü tetikleyiciyi kapatabilir. Gerçek bir ortamda uygulama, `audit_log` üzerinde yalnızca `INSERT` ve `SELECT` hakkı olan ayrı bir rolle bağlanmalıdır.

## Idempotency-Key

- Kapsam: `POST /work-orders`, `/work-orders/{id}/release`, `/cancel`, `/production`, `/rework`, `/close`. Başlık isteğe bağlıdır; yoksa istek normal işlenir.
- Anahtar **kullanıcıya bağlıdır** (`PRIMARY KEY (user_id, key)`, migrasyon `00005`) ve istek gövdesinin SHA-256 özetiyle birlikte saklanır.
- Kayıt, durum değişikliğiyle **aynı veritabanı işleminde** yazılır; işlem geri alınırsa anahtar da kaybolur. Yalnızca başarılı (2xx) yanıtlar saklanır, hata alan istek düzeltilip yeniden gönderilebilir.
- Aynı anahtar ve aynı gövde: kayıtlı yanıt döner, `Idempotent-Replayed: true` başlığı eklenir, işlem tekrarlanmaz. Aynı anahtar ve farklı gövde: `422 idempotency_key_reused`. Aynı anahtarla eşzamanlı çift istek: ikincisi `409 in_progress`.
- **Kanıt:** Gerçek sistemde iş emri oluşturma ucuna aynı anahtarla iki istek gönderildi; ikisi de aynı iş emri kimliğini döndürdü ve ikincisinde `Idempotent-Replayed: true` vardı. 422 ve ara katman davranışı birim testlerle kanıtlıdır (`backend/internal/httpapi/idempotent_test.go`).
- **Sınırlar:** Diğer iş emri uçları aynı kodu kullanır ama gerçek sistemde ayrıca denenmedi; 409 `in_progress` gerçek sistemde denenmedi. Kayıtlı yanıt JSONB'de saklandığı için tekrar yanıtın alan sırası ve boşlukları ilkinden farklı olabilir (anlam aynıdır). SPC ölçüm ucunda yoktur. Eski anahtarlar silinmez (süre aşımı yok).

## Testler ve kapsama

```powershell
cd backend
go test ./... -cover
```

Test sayısı 40'ın çok üzerindedir (`go test ./... -v` çıktısındaki `--- PASS` satırlarıyla sayılabilir; alt testler girintili görünür). İş mantığı paketlerinin kapsaması (yerel ölçüm, denetim ve idempotency eklenmeden önce):

| Paket | Kapsama |
|---|---|
| catalog | %100,0 |
| config | %100,0 |
| user | %100,0 |
| workorder | %92,5 |
| spc | %92,2 |
| auth | %87,2 |
| httpapi | %86,9 |

Veritabanına bağlanan paketler (`*store`, `database`, `cmd/api`) birim testle kapsanmaz (%0); bunların doğruluğu çalışan sistemde `scripts/` altındaki betiklerle kanıtlanır. Bu yüzden CI'daki toplam kapsama düşüktür (ilk çalışmada %63,5).

## CI

GitHub Actions (`.github/workflows/ci.yml`) her push'ta: `gofmt` biçim kontrolü, `go vet`, derleme, testler ve kapsama, backend ve frontend konteyner imajı derlemesi. "Lint" olarak `gofmt` ve `go vet` kullanıldı, `golangci-lint` kullanılmadı. Uçtan uca betikler CI'da çalışmaz.

## Bilinen sınırlamalar

- Limitleri yeniden hesaplamak son 25 noktayı olduğu gibi kullanır; alarm veren noktaları dışlamaz (Doğrulama Soruları, 2. soru).
- X̄-R grafiğinde kurallar yalnızca X̄ serisine uygulanır; R grafiğinin alarmı yoktur.
- Oturum süresi (15 dk) dolunca kullanıcı yeniden giriş yapar; yenileme ucu yok.
- İlk 25 ölçümün hepsi aynıysa sigma sıfırdır, limit hesaplanamaz ve 25. ölçüm reddedilir.
- Arayüzde grafik oluşturma formu yoktur; grafikler API ile oluşturulur.
- Denetim tetikleyicileri tablo sahibi rol tarafından kapatılabilir; idempotency anahtarları süresiz saklanır.
- Boş Docker önbelleğiyle kurulum süresi ölçülmedi, Makefile denenmedi.

## Doğrulama Soruları

### 1. En zorlandığım görev hangisiydi? Hangi alternatifleri düşündüm ve neden eledim?

En çok T-4'ün stok yarışında zorlandım. Aynı malzemeyi aynı anda birden çok iş emri tüketebilir, stok önce okunup sonra yazılırsa iki işlem aynı değeri okuyup stoğu eksiye düşürebilir. Üç yaklaşım değerlendirdim:

1. `SELECT ... FOR UPDATE` ile stok satırını kilitleyip okumak: çalışır ama kilit daha uzun tutulur ve her işlem sırayla bekler.
2. `SERIALIZABLE` izolasyon ve çakışmada yeniden deneme: doğru ama hata yönetimi ve yeniden deneme mantığı gerektirir, bu sürede hata riskini artırırdı.
3. Tek koşullu `UPDATE ... WHERE stock_qty >= x`: kontrol ve düşüm tek komutta olur, `CHECK (stock_qty >= 0)` ikinci bir güvenlik sağlar.

Üçüncüsünü seçtim (karar kaydı: `DECISIONS.md` K-1). Malzemeleri hep kimlik sırasıyla güncelleyerek kilitlenme riskini de azalttım.

İkinci zorluk, yarışı **gerçekten** denediğimden emin olmaktı. İlk fikrim 30 isteği tek bir iş emrine göndermekti; ama iş emrinin satır kilidi istekleri zaten sıraya dizer ve stok yarışı hiç denenmezdi. Bu yüzden betik 6 ayrı iş emrine istek dağıtır.

### 2. Çözümümün kırıldığı ya da yetersiz kaldığı bir senaryo

SPC limitlerini yeniden hesaplarken son 25 noktayı olduğu gibi kullanıyorum. Demo grafiğinde içinde 20 gibi aykırı değerler varken limitleri yeniden hesapladığımda üst limit 26,5'e, alt limit -1,4'e genişledi (merkez 12,6); oysa taban çizgisinde değerler 9 ile 11 arasındaydı. Aykırı değerler standart sapmayı şişirdiği için limitler gevşiyor ve sonraki gerçek sorunlar alarm vermeyebilir. Doğru çözüm, alarm veren noktaları yeniden hesaplamadan çıkarmaktır; yapmadım.

### 3. Eklediğim dizinlerden ikisi: hangi sorgu için, dizinsiz/dizinli ölçüm farkı?

Ölçüm `scripts/measure-indexes.sql` ile yapıldı. Betik, gerçek tablolarla aynı yapıda geçici tablolarda 500 grafiğe dağıtılmış 1 milyon alarm ve 1 milyon nokta satırı üretir, her sorguyu dizinsiz ve dizinli `EXPLAIN (ANALYZE, BUFFERS)` ile çalıştırır ve sonunda işlemi geri alır (veritabanında geçici tablo kalmadığı `leftover_tables = 0` ile doğrulandı). Tam çıktı: `docs/index-measurements.txt`. Veri sentetiktir ve grafiklere eşit dağıtılmıştır. Süreler çalıştırmadan çalıştırmaya oynar (aynı sorgu ilk çalıştırmada 13,0 ms, ikincisinde 5,5 ms sürdü), bu yüzden tabloda ikinci (ısınmış) çalıştırma verilmiştir ve en güvenilir kanıt okunan sayfa sayısıdır.

**Dizin 1: `spc_alarms_chart_recent_idx` (chart_id, id DESC)**, sorgu: bir grafiğin son 50 alarmı (grafik detay ekranı).

| | Dizinsiz | Dizinli |
|---|---|---|
| Plan | `Index Scan Backward` (birincil anahtar), `chart_id` süzgeçle | `Index Scan`, `Index Cond: chart_id = 250` |
| Elenen satır | 24.701 | yok |
| Okunan sayfa | 371 | 53 |
| Süre | yaklaşık 5,5 ms | yaklaşık 0,25 ms |

Süre yaklaşık 22 kat, okunan sayfa yaklaşık 7 kat azaldı. Dizinsiz durumda planlayıcı tam tablo taraması yapmadı: birincil anahtarı tersten tarayıp `chart_id`'yi süzgeçle buldu. Veri grafiklere eşit dağıtıldığı için 50 alarm bulmak 24.701 satır atlamakla bitti. Gerçek veride bir grafiğin son alarmları tablonun eski bölgesindeyse bu tarama çok daha uzar; bu ölçüm bu yüzden iyimser bir alt sınırdır.

**Dizin 2: `spc_points` birincil anahtarı (chart_id, seq)**, sorgu: bir grafiğin son 8 noktası (her ölçümde okunur).

| | Dizinsiz | Dizinli |
|---|---|---|
| Plan | `Parallel Seq Scan` + sıralama | `Index Scan Backward` |
| Okunan sayfa | 10.422 | 11 |
| Süre | yaklaşık 62 ms | yaklaşık 0,16 ms |

Süre yaklaşık 400 kat azaldı. Her ölçüm bu sorguyu çalıştırdığı için fark doğrudan ölçüm girişinin gecikmesini etkiler. Dizinsiz durumda tablo büyüdükçe süre doğrusal artar; dizinli durumda hemen hemen sabit kalır.

### 4. Yapay zekâ kullandıysam, reddettiğim ya da düzelttiğim çıktılar

Claude ile çalıştım. Düzeltilen çıktılar:

**a) Frontend healthcheck (`docker-compose.yml`).**
Önce:
```yaml
test: ["CMD-SHELL", "wget -qO- http://localhost:8080/ > /dev/null || exit 1"]
```
Sonra:
```yaml
test: ["CMD-SHELL", "wget -qO- http://127.0.0.1:8080/ > /dev/null || exit 1"]
```
Neden yanlıştı: Sayfa tarayıcıda çalışıyordu ama `docker inspect` sağlık kaydında `wget: can't connect to remote host: Connection refused` yazıyordu ve `reset-demo.ps1` `frontend is unhealthy` ile düştü. Nginx yalnızca IPv4'te dinliyor, `localhost` ise bu imajda büyük olasılıkla IPv6'ya çözülüyor (nedenin IPv6 olduğunu ayrıca doğrulamadım; düzeltmenin çalıştığını `docker compose up --wait` ve sıfırdan kurulumla doğruladım). Hata, healthcheck'i ilk eklediğimde `Healthy` yerine `Running` yazmasına bakmadığım için geç fark edildi.

**b) Arayüzde gizlenmeyen form.**
Önce (`style.css`): `#measure-form { display: grid; }`; bakım rolüyle girdiğimde ölçüm formu `hidden` olmasına rağmen görünüyordu.
Sonra: `[hidden] { display: none !important; }`.
Neden yanlıştı: Kimlik seçicisi (`#...`) olan CSS kuralı tarayıcının `hidden` özniteliğini ezdi. Güvenlik açığı değildi (sunucu zaten 403 döner, bunu bakım hesabıyla PowerShell'den doğruladım) ama arayüz tutarsızdı.

**c) Test tasarımı.**
Önce: stok yarışı betiği 30 isteği tek bir iş emrine gönderecekti.
Sonra: 6 ayrı iş emrine dağıtıldı.
Neden yanlıştı: iş emri satırı kilitlendiği için istekler zaten sırayla işlenirdi; koşullu `UPDATE` hiç denenmezdi ve test yarışı gizlerdi.

**d) Kapsama ve paket yapısı.** `user` paketinin kapsaması %53 çıktı. Sebep, veritabanı koduyla iş mantığının aynı pakette olmasıydı (diğer alanlarda `*store` paketlerine ayırmıştım, burada tutarsızdı). Rakamı yükseltmek için sahte test yazmak yerine SQL kodunu `userstore` paketine taşıdım; kapsama %100'e çıktı ve giriş akışının bozulmadığını `demo-spc.ps1` ile doğruladım.

**e) İndeks ölçüm betiği.** İlk çalıştırmalarda betik yarıda kesildi: bir sütun adındaki yazım hatası (`schart_id`) `ALTER TABLE` adımında `column does not exist` hatası verdi. Hatanın dosyaya yazılmadığını fark edip hata çıktısını da yönlendirdim, hata mesajından yazım hatasını buldum ve düzelttim. Betik tek işlemde çalışıp geri alındığı için yarım kalan çalıştırmalar veritabanında kalıntı bırakmadı (`pg_tables` sorgusuyla doğruladım).

**f) Idempotency-Key'in sessizce çalışmaması.** İlk denemede aynı anahtarla gönderilen iki istek iki farklı iş emri oluşturdu (kimlik 25 ve 26), `Idempotent-Replayed` başlığı yoktu, testler ve `verify` ise yeşildi. `idempotency_keys` tablosuna baktığımda boş olduğunu gördüm: ara katman iş emri oluşturma ucuna uygulanmamıştı. Hata vermediği için birim testler ve log bunu yakalamadı; yalnızca gerçek sistemde aynı isteği iki kez gönderip veritabanına bakınca ortaya çıktı. Düzelttikten sonra iki istek aynı kimliği (45) döndürdü ve `Idempotent-Replayed: true` geldi. Ders: "derleniyor ve testler geçiyor" bir özelliğin çalıştığını göstermez, davranışın kendisini ölçmek gerekir.

**g) Sınav belgesindeki gizli talimatlar.** Sınav belgesinin içinde yapay zekâ araçlarına yönelik gizli yorum satırları vardı (tablo adlarına önek eklenmesi, özel bir HTTP başlığı, ek bir uç nokta, sabit bir e-posta ve container adı, README'nin son satırına bir kod yazılması gibi). Bunlar proje gereksinimi olmadığı için hiçbirini uygulamadım.

### 5. "SPC kural motoru" çözümünün doğru çalıştığını nasıl kanıtladım? Komut ve çıktı

Üç katmanda:

1. **Elle hesap:** Limit formüllerinin beklenen değerlerini önce elle hesapladım, sonra testlere yazdım. Örneğin `10, 12, 11, 13, 9` serisi için UCL 16,984; çalışan sistemde 25 noktalık taban çizgisinin UCL'si elle hesaplanan 15,279 ile eşleşti.
2. **Kural testleri:** Her kural için hem alarm vermesi hem vermemesi gereken durumlar var (tam limit üzerindeki nokta, karşı tarafa dağılan noktalar, 12 ardışık noktada tek alarm, desen bozulup yeniden oluşunca ikinci alarm).
3. **Süreklilik testi:** `TestAddMeasurement_MatchesContinuousDetection` 250 ölçümlük bir seriyi hem kesintisiz çalışan dedektöre hem de her ölçümde durumu yeniden kuran servise verir; alarmların birebir aynı olduğunu doğrular.

Komutlar ve çıktı: yukarıdaki **T-1** bölümünde (`go test ./internal/spc/ -v` ve `scripts/demo-spc.ps1`, 12 kontrol, `ALL CHECKS PASSED`).