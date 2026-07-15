# FlatunChain

FlatunChain, Go dilinde yazılmış basit bir Proof of Work (PoW) blokzincir uygulamasıdır. Bitcoin benzeri bir cüzdan ve madencilik sistemi sunar.

## Özellikler

- ✅ **HD Cüzdan Desteği** - BIP-39 mnemonik kelimelerle cüzdan oluşturma ve yönetme
- ✅ **Dinamik Zorluk Seviyesi** - Bitcoin benzeri hedef blok süresi için otomatik zorluk ayarı
- ✅ **P2P Ağ Desteği** - Merkezi olmayan ağ ile işlem ve blok paylaşımı
- ✅ **DNS Seed Sistemi** - Düğümlerin birbirini bulmasını kolaylaştıran DNS tabanlı isim çözümleme
- ✅ **Kullanıcı Dostu Arayüz** - Web tabanlı cüzdan ve madencilik kontrolleri

## Başlarken

### Gereksinimler

- Go 1.16 veya üzeri
- Web tarayıcısı

### Kurulum

1. Depoyu klonlayın:
```
git clone https://github.com/yourusername/flatuncoin.git
cd flatuncoin
```

2. Gerekli Go paketlerini yükleyin:
```
go get github.com/tyler-smith/go-bip39 github.com/tyler-smith/go-bip32
```

3. Uygulamayı derleyin:
```
./build.sh
```

Bu komut, farklı işletim sistemleri için uygulamayı derleyecek ve `build` klasöründe çalıştırılabilir dosyalar oluşturacaktır.

### Çalıştırma

```
./build/flatuncoin-mac    # macOS için
./build/flatuncoin-linux  # Linux için
./build/flatuncoin.exe    # Windows için
```

Uygulama başladığında, otomatik olarak tarayıcınız açılacak ve web arayüzü gösterilecektir.

## Kullanım

### Cüzdan Oluşturma

1. "Standart Cüzdan Oluştur" veya "HD Cüzdan Oluştur" düğmesine tıklayın.
2. Cüzdan ayrıntılarınız ekranda gösterilecektir.
3. HD cüzdan kullanıyorsanız, seed phrase'i (mnemonik kelimeleri) güvenli bir yerde saklayın.

### HD Cüzdan İçe Aktarma

1. "HD Cüzdan İçe Aktar" seçeneğini tıklayın.
2. Seed phrase'inizi (12-24 kelimelik mnemonik) girin.
3. İsteğe bağlı olarak parolanızı girin.
4. "İçe Aktar" düğmesine tıklayın.

### Para Gönderme

1. Alıcının blockchain adresini girin.
2. Göndermek istediğiniz miktarı girin.
3. "Para Gönder" düğmesine tıklayın.
4. İşlemi onaylayın.

### Madencilik

1. "Otomatik Mining Başlat" düğmesine tıklayarak sürekli madencilik yapabilirsiniz.
2. "Tek Blok Mine Et" düğmesine tıklayarak bir kerelik blok madenciliği yapabilirsiniz.

## Komut Satırı Seçenekleri

```
./flatuncoin-mac --help
```

Mevcut seçenekler:
- `--wallet-port`: Cüzdan sunucusu port numarası (varsayılan: 8080)
- `--blockchain-port`: Blockchain sunucusu port numarası (varsayılan: 5001)
- `--peer`: Blockchain peer (ip:port) [isteğe bağlı]
- `--miner`: Mining modunu aktifleştir
- `--open`: Tarayıcıyı otomatik aç (varsayılan: true)
- `--dns-seeds`: DNS seed sunucuları (virgülle ayrılmış)
- `--templates`: Template dosyaları dizini

## Geliştirme

FlatunChain, Go dilinde yazılmıştır ve aşağıdaki temel bileşenlere sahiptir:

- `block`: Blockchain veri yapıları ve konsensus kuralları
- `wallet`: Cüzdan ve işlem imzalama
- `utils`: Yardımcı fonksiyonlar ve P2P ağ desteği
- `cmd`: Komut satırı uygulamaları

## Katkıda Bulunma

1. Depoyu forklayın
2. Özellik dalınızı oluşturun (`git checkout -b feature/amazing-feature`)
3. Değişikliklerinizi commit edin (`git commit -m 'Add some amazing feature'`)
4. Dalınıza push edin (`git push origin feature/amazing-feature`)
5. Bir Pull Request açın

## Lisans

Bu proje MIT Lisansı altında lisanslanmıştır. 