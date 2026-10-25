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