# Mimari Karar Kayıtları

Her kayıtta bağlam, karar, düşünülen alternatifler ve kararın bedeli yazılıdır.

## K-1: Stok düşümü koşullu UPDATE ile yapılır (T-4)

**Bağlam:** Farklı iş emirleri aynı malzemeyi aynı anda tüketebilir. Stok önce okunup sonra yazılırsa iki işlem aynı değeri okuyup ikisi de düşüm yapabilir ve stok eksiye düşer.

**Karar:** Düşüm tek bir komutla yapılır: `UPDATE materials SET stock_qty = stock_qty - x WHERE id = ? AND stock_qty >= x`. Etkilenen satır sayısı 0 ise stok yetmiyordur ve işlem geri alınır. Tabloda ayrıca `CHECK (stock_qty >= 0)` kısıtı vardır. Malzemeler hep kimlik sırasıyla güncellenir, böylece iki işlem satırları ters sırada kilitleyip birbirini bekleyemez.

**Alternatifler:** `SELECT ... FOR UPDATE` ile stok satırını kilitleyip okumak (daha uzun kilit süresi), `SERIALIZABLE` izolasyon ve yeniden deneme (daha karmaşık, hata yönetimi gerektirir).

**Bedeli:** Kontrol ve düşüm aynı komutta olduğu için "neden yetmedi" ayrıntısı (mevcut stok kaç) hata mesajında yoktur. Kanıt: `scripts/demo-stock-race.ps1` içinde 6 iş emrinden 30 paralel istek gelir, tam 20'si başarılı olur, stok 0'a iner ve eksiye düşmez.

## K-2: İş emri durum makinesi saf pakettir, tutarlılık iki yerde korunur (T-4)

**Bağlam:** İş emrinin sayaçları (başlayan, sağlam, fire, yeniden işlemde bekleyen) birbiriyle tutarlı kalmalıdır.

**Karar:** Durum geçişleri ve miktar kuralları `workorder` paketinde, veritabanı ve HTTP bilmeden yazılmıştır. Değişmez kural: `başlayan = sağlam + fire + bekleyen`. Bu kural kodda her işlemden sonra denetlenir ve veritabanında `CHECK` kısıtıdır. Başlamış bir iş emri iptal edilemez, çünkü malzeme zaten düşmüştür; bunun yerine açık onayla (`allow_partial`) kısmi kapanış yapılır. Kapanışta tüketilen malzeme, "başlayan adet × reçete miktarı" ile karşılaştırılır.

**Alternatifler:** Kuralları sadece veritabanında tutmak (hata mesajları anlaşılmaz olur, test zorlaşır), iptalde malzemeyi iade etmek (iade edilecek malzeme fiziksel olarak harcanmış olabilir).

**Bedeli:** Miktarlar tam sayıdır, kesirli birimler (kg, metre) desteklenmez.

## K-3: SPC dedektörünün durumu her ölçümde son noktalardan yeniden kurulur (T-1)

**Bağlam:** Kural motoru önceki noktaları bilmek zorundadır. Bellekte tutulan durum, sunucu yeniden başladığında ya da birden çok kopya çalıştığında kaybolur ya da çelişir.

**Karar:** Her ölçümde grafik satırı `SELECT ... FOR UPDATE` ile kilitlenir, son 8 nokta okunur, dedektöre sessizce yedirilir ve yeni nokta ondan sonra eklenir. 8, en uzun kuralın (8 ardışık nokta) penceresidir. Nokta, limit ve alarm aynı veritabanı işleminde yazılır.

**Alternatifler:** Dedektörü bellekte tutmak (yeniden başlatmada tekrar alarm riski), durumu ayrı bir tabloda saklamak (ikinci bir doğruluk kaynağı).

**Bedeli:** Her ölçüm bir miktar fazladan okuma yapar. Doğruluğu `TestAddMeasurement_MatchesContinuousDetection` kanıtlar: 250 ölçümlük bir seride, her ölçümde durumu yeniden kuran servis ile kesintisiz çalışan dedektör birebir aynı alarmları üretir.

## K-4: Tekrar alarm iki katmanda engellenir (T-1)

**Bağlam:** Aynı ihlal için tekrar alarm üretilmemelidir. Örneğin merkezin aynı tarafında 12 ardışık nokta tek alarm vermelidir.

**Karar:** Desen kuralları, ihlal başladığında bir kez alarm verir ve ihlal sürdüğü sürece yenisini üretmez, desen bozulup yeniden oluşursa tekrar alarm verir. Buna ek olarak `spc_alarms` tablosunda `UNIQUE (chart_id, point_seq, rule)` kısıtı vardır. Tek nokta kuralı (3 sigma dışı) her noktayı ayrı bir olay sayar.

**Alternatifler:** Yalnızca uygulama katmanında kontrol (bir hata durumunda sessizce çift kayıt), yalnızca veritabanı kısıtı (alarm üretilip sonra reddedilir, gereksiz iş).

**Bedeli:** İki yerde kural bakımı gerekir.

## K-5: Yetki kontrolü sunucuda, ara katmanlarla yapılır

**Bağlam:** Belge, her uç noktada yetki kontrolü ve yetkisiz erişimin sunucu tarafında engellenmesini ister.

**Karar:** Her korumalı uç `Authenticate` (token geçerli mi, access türünde mi) ve `RequireRoles` (rol izinli mi) ara katmanlarından geçer. Token yok ya da geçersizse 401, rol yetersizse 403 döner. Refresh token ile access token yerine geçilemez. Rol-uç eşlemesi her API'nin `Register` fonksiyonunda tek bir yerde durur ve testler tüm rol matrisini dener. Arayüz, yetkisi olmayan rollerde formları gizler ama bu yalnızca kullanım kolaylığıdır, güvenlik sunucudadır.

**Alternatifler:** Rol kontrolünü her işleyicinin içinde yapmak (unutulma riski), arayüzde gizlemeye güvenmek (güvenlik sağlamaz).

**Bedeli:** Rol kümeleri kodda sabittir. Yeni bir rol ya da yetki eklemek kod değişikliği gerektirir.

## K-6: Go standart kütüphanesi, ek web çatısı yok

**Bağlam:** Belge HTTP katmanı için serbest seçim bırakır.

**Karar:** Go 1.22 ile gelen yöntem ve yol kalıplı yönlendirme (`"POST /work-orders/{id}/close"`) kullanıldı. Bağımlılıklar az tutuldu: pgx, goose, bcrypt ve JWT kütüphanesi.

**Alternatifler:** chi, gin, echo.

**Bedeli:** Ara katman zinciri ve ortak yardımcılar elle yazıldı. Süre kısıtlı olduğu için bunu bilinçli kabul ettim.

## K-7: Frontend düz HTML, CSS ve JavaScript, nginx ile sunulur

**Bağlam:** Teslim süresi kısa. Frontend serbest ama çalışan ve her ekranda yükleniyor, boş ve hata durumlarını gösteren bir arayüz gerekir.

**Karar:** Derleme adımı olmayan düz bir arayüz seçildi, root olmayan nginx imajıyla sunulur ve `/api/` isteklerini backend'e yönlendirir (böylece tarayıcıda CORS sorunu yoktur). Kontrol grafiği kütüphane kullanmadan SVG ile çizilir. Sunucudan gelen metinler `textContent` ile yazılır, HTML olarak yorumlanmaz.

**Alternatifler:** React ya da Vue (derleme ve bağımlılık riski), grafik kütüphanesi (ek bağımlılık).

**Bedeli:** Büyük bir arayüzde bileşen yapısı olmadığı için bakım zorlaşırdı. Oturum token'ı `sessionStorage` içinde tutulur ve süre dolunca kullanıcı yeniden giriş yapar, çünkü yenileme (refresh) ucu henüz yazılmadı.

## K-8: Test stratejisi ve CI'daki "lint" kapsamı

**Bağlam:** Süre kısıtlı olduğu için hem hızlı hem güvenilir bir doğrulama gerekiyordu.

**Karar:** İş mantığı arayüzler üzerinden sahte depolarla birim testlenir. Gerçek PostgreSQL davranışı (kilitler, kısıtlar, eşzamanlılık) çalışan sistem üzerinde `scripts/` altındaki uçtan uca betiklerle kanıtlanır. CI'da lint olarak `gofmt` ve `go vet` çalışır.

**Alternatifler:** Testcontainers ile Go entegrasyon testleri (kurulum süresi ve CI karmaşıklığı), `golangci-lint` (görmediğim kural setiyle CI'ı kırma riski).

**Bedeli:** Veritabanı katmanı (`*store` paketleri) birim testle değil betikle kapsanır, bu yüzden paket bazında kapsama oranları düşük görünür. Betikler CI'da otomatik çalışmaz, elle çalıştırılır.

## K-9: OEE saklanmaz, her sorguda olaylardan hesaplanır

**Bağlam:** Duruş olayları 30 dakika geç gelebilir; geçmiş vardiyanın OEE değeri o anda doğru olmalıdır.

**Alternatifler:**
1. Vardiya sonunda OEE'yi hesaplayıp tabloya yazmak, geç olayda yeniden hesap işi tetiklemek.
2. Olay geldikçe artımlı sayaçları güncellemek.
3. OEE'yi saklamamak, her sorguda o güne ait olaylardan yeniden hesaplamak.

**Karar:** 3. Hesap saf bir fonksiyondur (`internal/oee`), aynı olaylar hangi sırayla gelirse gelsin aynı sonucu verir. Geç olay yazıldığı anda sonuç düzelir, ayrı bir yeniden hesap işine ve tutarsız ara duruma gerek kalmaz.

**Bedeli:** Her sorguda bir günün olayları okunur. Bir hat için günde birkaç bin olayda sorun değildir; çok uzun dönemli raporlar için önceden hesaplanmış özetler gerekir.

## K-10: Tekrar gelen olaylar veritabanında tekilleştirilir

**Bağlam:** Simülatör ve gerçek cihazlar aynı olayı tekrar gönderebilir; sayaç iki kez sayılırsa OEE yanlış olur.

**Karar:** Hat sayacında `(line_id, occurred_at)`, duruşta `(line_id, reason_code, started_at, ended_at)` benzersizdir; `INSERT ... ON CONFLICT DO NOTHING` ile tekrar yutulur ve mevcut kaydın kimliği döner.

**Bedeli:** Aynı anda gelen iki gerçek sayaç kaydı tek kayıt sayılır; gerçek bir sistemde olaylar cihazdan gelen bir olay kimliğiyle tekilleştirilmelidir.

## K-11: Soy ağacı PostgreSQL'de kenar tablosu ve recursive CTE ile

**Alternatifler:** Ayrı bir graf veritabanı; her parti için tüm atalarını tutan düzleştirilmiş tablo (closure table); kenar tablosu ve `WITH RECURSIVE`.

**Karar:** Kenar tablosu ve `WITH RECURSIVE`, iki yön için ayrı dizin, 20 seviye sınırı. Ek bir altyapı gerektirmez; 1 milyon partide sorgular 1 ms'nin altında kaldı.

**Bedeli:** Çok derin ya da çok geniş ağaçlarda sorgu süresi büyür; o durumda closure table düşünülmelidir.

## K-12: Sensör verisi aylık partisyonlu tablo ve COPY

**Alternatifler:** Tek tablo ve satır satır `INSERT`; zaman serisi veritabanı (TimescaleDB); yerel partisyon ve `COPY`.

**Karar:** Aylık `RANGE` partisyonu ve toplu `COPY`. Eski veri, partisyonu ayırıp arşiv şemasına taşıyarak saniyeler içinde arşivlenir; sorgular partisyon budamayla yalnızca ilgili ayı tarar. Ek bir eklenti gerektirmez.

**Bedeli:** Partisyonları oluşturan ve arşivleyen fonksiyonları kendimiz yönetiyoruz. İlk yük testinde partisyon oluşturma sırasında bir yarış görüldü ve düzeltildi (README, soru 4g).

## K-13: Denetim kaydı veritabanı tetikleyicileriyle

**Alternatifler:** Her depoda uygulama koduyla kayıt yazmak; tetikleyiciyle yazmak.

**Karar:** Tetikleyici. Stok düşümü dahil hiçbir değişiklik unutulamaz, eski ve yeni değer eksiksiz gelir. "Kim" bilgisi işlem başında `app.user_id` oturum ayarına yazılır. Tablo `UPDATE`, `DELETE` ve `TRUNCATE`'e kapalıdır.

**Bedeli:** Tablo sahibi rol tetikleyiciyi kapatabilir; gerçek ortamda uygulama ayrı ve kısıtlı bir rolle bağlanmalıdır.

## K-14: Idempotency kaydı iş işlemiyle aynı transaction'da

**Alternatif:** Önce işlemi yapıp sonra anahtarı ayrı işlemde kaydetmek.

**Karar:** Anahtar ve yanıt, durum değişikliğiyle aynı işlemde yazılır. İşlem geri alınırsa anahtar da kaybolur; yarım kalmış bir işlem "yapıldı" sayılmaz, sunucu çökse bile tekrar istek doğru davranır.

**Bedeli:** Her depo metodunun idempotency kaydını bilmesi gerekir; bu yüzden şimdilik yalnızca iş emri uçlarında var.

## K-15: Simülatör API üzerinden yazar

**Alternatif:** Simülatörün doğrudan veritabanına yazması.

**Karar:** Simülatör, demo hesaplarla giriş yapıp gerçek API'yi kullanır. Böylece doğrulama, yetki, tekilleştirme ve SPC kuralları gerçek akışta sürekli çalışır; simülatör aynı zamanda sürekli çalışan bir uçtan uca testtir.

**Bedeli:** Simülatör API'ye bağımlıdır, backend sağlıklı olmadan başlamaz (compose `depends_on` ile çözülür).