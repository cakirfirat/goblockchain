// electron-builder afterPack kancası (yalnızca macOS).
//
// Neden: Developer ID imzası olmadan paketlenen uygulamada yalnızca linker'ın
// otomatik imzası kalıyor ve kaynaklar mühürsüz oluyor. Tarayıcıdan indirilen
// (karantinalı) kopyada Gatekeeper bunu "uygulama hasarlı, çöpe taşıyın" diye
// gösterir ve hiçbir açma yolu sunmaz. Geçerli bir ad-hoc imza ile mesaj
// "geliştirici doğrulanamadı"ya döner ve Sistem Ayarları > Gizlilik ve
// Güvenlik'te "Yine de Aç" seçeneği çıkar.
//
// Apple kimlik bilgileri (release.env) tanımlandığında bu kanca kendini devre
// dışı bırakır; gerçek imza + notarization akışı devreye girer.
const { execFileSync } = require('child_process');
const path = require('path');

module.exports = async function adhocSign(context) {
  if (context.electronPlatformName !== 'darwin') return;
  if (process.env.APPLE_ID && process.env.APPLE_APP_SPECIFIC_PASSWORD && process.env.APPLE_TEAM_ID) {
    return; // gerçek imza akışı bunu ezecek
  }
  const appPath = path.join(context.appOutDir, `${context.packager.appInfo.productFilename}.app`);
  execFileSync('codesign', ['--force', '--deep', '--sign', '-', appPath], { stdio: 'inherit' });
  execFileSync('codesign', ['--verify', '--deep', '--strict', appPath], { stdio: 'inherit' });
  console.log('  • ad-hoc imza uygulandı (Developer ID gelene dek geçici yol)');
};
