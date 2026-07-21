#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# FlatunChain Sürüm Hattı (release pipeline)
#
# Tek komutla: testler -> Go sidecar'lar (3 platform) -> Electron paketleri
# (dmg/exe/AppImage) -> SHA256 checksum + latest.json -> sunucuya atomik yayın.
#
# Tetikleyici sensin: ./release.sh
# Anahtarlar (imza, SSH) hiçbir zaman bu makineden çıkmaz; sunucu yalnızca
# statik dosya servis eder.
#
# Kullanım:
#   ./release.sh                        # test + build + yayın
#   ./release.sh --bump patch          # önce sürümü artır (patch|minor|major)
#   ./release.sh --no-upload           # yalnızca build + stage (yayın yok)
#   ./release.sh --publish-only        # mevcut stage'i yayınla (build yok)
#   ./release.sh --skip-tests          # testleri atla (önerilmez)
#   ./release.sh --target /tmp/deneme  # yerel dizine yayınla (prova için)
#   ./release.sh --force               # sunucuda aynı sürüm varsa üzerine yaz
#
# Yapılandırma release.env dosyasıyla ezilebilir (gitignore'da):
#   DEPLOY_TARGET=root@159.89.31.131
#   REMOTE_DIR=/var/www/flatun-downloads
#   BASE_URL=https://downloads.yoxar.com
#   APPLE_ID=... APPLE_APP_SPECIFIC_PASSWORD=... APPLE_TEAM_ID=...  (imza gelince)
# ============================================================================

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
log()  { echo -e "${GREEN}[release]${NC} $*"; }
warn() { echo -e "${YELLOW}[release] UYARI:${NC} $*"; }
die()  { echo -e "${RED}[release] HATA:${NC} $*" >&2; exit 1; }

# --- Varsayılan yapılandırma ---
DEPLOY_TARGET="${DEPLOY_TARGET:-root@159.89.31.131}"
REMOTE_DIR="${REMOTE_DIR:-/var/www/flatun-downloads}"
BASE_URL="${BASE_URL:-https://downloads.yoxar.com}"
# shellcheck disable=SC1091
[ -f "$ROOT/release.env" ] && source "$ROOT/release.env"

# --- Bayraklar ---
BUMP=""; NO_UPLOAD=0; PUBLISH_ONLY=0; SKIP_TESTS=0; FORCE=0
while [ $# -gt 0 ]; do
  case "$1" in
    --bump)         BUMP="${2:-}"; shift 2 ;;
    --no-upload)    NO_UPLOAD=1; shift ;;
    --publish-only) PUBLISH_ONLY=1; shift ;;
    --skip-tests)   SKIP_TESTS=1; shift ;;
    --target)       DEPLOY_TARGET="${2:-}"; shift 2 ;;
    --force)        FORCE=1; shift ;;
    -h|--help)      sed -n '3,30p' "$0"; exit 0 ;;
    *) die "bilinmeyen bayrak: $1 (yardım için --help)" ;;
  esac
done

# Hedef yerel bir dizin mi, SSH hostu mu?
LOCAL_TARGET=0
case "$DEPLOY_TARGET" in
  /*|./*|../*) LOCAL_TARGET=1; REMOTE_DIR="$DEPLOY_TARGET" ;;
esac

command -v node  >/dev/null || die "node bulunamadı"
command -v go    >/dev/null || die "go bulunamadı"
command -v rsync >/dev/null || die "rsync bulunamadı"

# --- Sürüm ---
if [ -n "$BUMP" ]; then
  [ "$PUBLISH_ONLY" = 1 ] && die "--bump ile --publish-only birlikte kullanılamaz"
  (cd desktop && npm version "$BUMP" --no-git-tag-version >/dev/null)
  log "Sürüm artırıldı ($BUMP)"
fi
VERSION="$(node -p "require('./desktop/package.json').version")"
STAGE="$ROOT/release/v$VERSION"
log "Sürüm: v$VERSION"

if ! git diff --quiet || ! git diff --cached --quiet; then
  warn "çalışma ağacında commit'lenmemiş değişiklikler var — sürüm bunlarla derlenecek"
fi

# Stage'deki artefaktları bulur (DMG/EXE/APPIMAGE değişkenlerini doldurur).
# dist/ içinde eski sürümler kalabildiği için desen sürüme sabitlenir.
resolve_artifacts() {
  local dir="$1"
  DMG=$(cd "$dir" && ls FlatunChain-"$VERSION"-*.dmg 2>/dev/null | head -1 || true)
  EXE=$(cd "$dir" && ls FlatunChain-Setup-"$VERSION"-*.exe 2>/dev/null | head -1 || true)
  APPIMAGE=$(cd "$dir" && ls FlatunChain-"$VERSION"-*.AppImage 2>/dev/null | head -1 || true)
  [ -n "$DMG" ] && [ -n "$EXE" ] && [ -n "$APPIMAGE" ] ||
    die "eksik artefakt ($dir içinde v$VERSION dmg/exe/AppImage üçlüsü bulunamadı)"
}

# ============================================================================
# 1) TEST + DERLEME
# ============================================================================
if [ "$PUBLISH_ONLY" = 0 ]; then

  if [ "$SKIP_TESTS" = 0 ]; then
    log "Go testleri çalışıyor..."
    go vet ./...
    go test ./... >/dev/null || { go test ./...; die "go testleri başarısız"; }
    log "Node testleri çalışıyor..."
    (cd desktop && npm test --silent >/dev/null) || die "desktop testleri başarısız"
  else
    warn "testler atlandı (--skip-tests)"
  fi

  log "Go binary'leri derleniyor (3 platform + sunucu)..."
  ./build.sh >/dev/null

  mkdir -p desktop/sidecar
  cp build/flatuncoin-mac build/flatuncoin-linux build/flatuncoin-windows.exe desktop/sidecar/

  # macOS imza/notarization: Apple kimlik bilgileri release.env'de tanımlıysa
  # electron-builder imzalar ve notarize eder; değilse imzasız (dev) paket üretir.
  if [ -n "${APPLE_ID:-}" ] && [ -n "${APPLE_APP_SPECIFIC_PASSWORD:-}" ] && [ -n "${APPLE_TEAM_ID:-}" ]; then
    export APPLE_ID APPLE_APP_SPECIFIC_PASSWORD APPLE_TEAM_ID
    log "macOS imza + notarization AKTİF (Team: $APPLE_TEAM_ID)"
  else
    export CSC_IDENTITY_AUTO_DISCOVERY=false
    warn "macOS paketi İMZASIZ üretilecek (release.env içinde APPLE_ID vb. yok)"
  fi
  # Windows imzası: Certum kartı geldiğinde bu noktaya signtool/jsign adımı
  # eklenecek; şimdilik imzasız.
  warn "Windows paketi İMZASIZ üretilecek (Certum sertifikası bekleniyor)"

  log "Electron paketleri derleniyor (dmg + nsis + AppImage)..."
  (cd desktop && npx electron-builder --mac --win --linux)

  # --- Stage: yayınlanacak dosyaları topla ---
  rm -rf "$STAGE" && mkdir -p "$STAGE"
  resolve_artifacts "$ROOT/desktop/dist"
  cp "desktop/dist/$DMG" "desktop/dist/$EXE" "desktop/dist/$APPIMAGE" "$STAGE/"
  (cd "$STAGE" && shasum -a 256 -- * > SHA256SUMS.txt)

  # latest.json: web sitesinin okuyacağı sürüm manifesti
  STAGE="$STAGE" BASE_URL="$BASE_URL" VERSION="$VERSION" node -e '
    const fs = require("fs"), path = require("path"), crypto = require("crypto");
    const { STAGE, BASE_URL, VERSION } = process.env;
    const plat = {
      ".dmg":      ["mac",     "macOS (Apple Silicon)", "arm64"],
      ".exe":      ["windows", "Windows 10/11 (64-bit)", "x64"],
      ".AppImage": ["linux",   "Linux (64-bit)",         "x86_64"],
    };
    const stable = {
      mac:     "FlatunChain-mac-arm64.dmg",
      windows: "FlatunChain-windows-x64.exe",
      linux:   "FlatunChain-linux-x86_64.AppImage",
    };
    const out = { product: "FlatunChain", version: VERSION,
                  publishedAt: new Date().toISOString(), platforms: {} };
    for (const f of fs.readdirSync(STAGE)) {
      const ext = f.endsWith(".AppImage") ? ".AppImage" : path.extname(f);
      if (!plat[ext]) continue;
      const [key, label, arch] = plat[ext];
      const buf = fs.readFileSync(path.join(STAGE, f));
      out.platforms[key] = {
        label, arch, file: f,
        url: `${BASE_URL}/releases/v${VERSION}/${f}`,
        stableUrl: `${BASE_URL}/latest/${stable[key]}`,
        sha256: crypto.createHash("sha256").update(buf).digest("hex"),
        size: buf.length,
      };
    }
    const keys = Object.keys(out.platforms);
    if (keys.length !== 3) { console.error("eksik platform:", keys); process.exit(1); }
    fs.writeFileSync(path.join(STAGE, "latest.json"), JSON.stringify(out, null, 2) + "\n");
  '
  log "Stage hazır: release/v$VERSION"
  ls -lh "$STAGE" | sed 1d
else
  [ -d "$STAGE" ] || die "stage yok: $STAGE (önce build çalıştırın)"
  resolve_artifacts "$STAGE"
fi

# ============================================================================
# 2) YAYIN — önce sürüm klasörü tam yüklenir, sonra 'latest' linkleri ve
#    latest.json ATOMİK olarak değiştirilir. Kullanıcı asla yarım sürüm görmez.
# ============================================================================
if [ "$NO_UPLOAD" = 1 ]; then
  log "--no-upload: yayın atlandı. Yayınlamak için: ./release.sh --publish-only"
  exit 0
fi

run_on_target() {  # hedefte komut çalıştır (yerel dizin veya SSH)
  if [ "$LOCAL_TARGET" = 1 ]; then bash -c "$1"; else ssh "$DEPLOY_TARGET" "$1"; fi
}

# Aynı sürüm zaten yayında mı?
if run_on_target "[ -e '$REMOTE_DIR/releases/v$VERSION' ]" 2>/dev/null; then
  [ "$FORCE" = 1 ] || die "v$VERSION sunucuda zaten var. Üzerine yazmak için --force, yeni sürüm için --bump kullanın."
  warn "v$VERSION üzerine yazılıyor (--force)"
fi

log "Yükleniyor -> $DEPLOY_TARGET:$REMOTE_DIR/releases/v$VERSION/"
run_on_target "mkdir -p '$REMOTE_DIR/releases'"
if [ "$LOCAL_TARGET" = 1 ]; then
  rsync -rt --delete "$STAGE/" "$REMOTE_DIR/releases/v$VERSION/"
else
  rsync -rt --delete --progress -e ssh "$STAGE/" "$DEPLOY_TARGET:$REMOTE_DIR/releases/v$VERSION/"
fi

log "'latest' linkleri ve latest.json atomik olarak güncelleniyor..."
run_on_target "set -e
  cd '$REMOTE_DIR'
  mkdir -p latest
  ln -sfn '../releases/v$VERSION/$DMG'      latest/FlatunChain-mac-arm64.dmg
  ln -sfn '../releases/v$VERSION/$EXE'      latest/FlatunChain-windows-x64.exe
  ln -sfn '../releases/v$VERSION/$APPIMAGE' latest/FlatunChain-linux-x86_64.AppImage
  ln -sfn '../releases/v$VERSION/SHA256SUMS.txt' latest/SHA256SUMS.txt
  cp 'releases/v$VERSION/latest.json' latest.json.tmp
  mv latest.json.tmp latest.json
"

log "YAYIN TAMAM: v$VERSION"
echo
echo "  Sabit linkler (web sitesine bir kez eklenir, bir daha değişmez):"
echo "    $BASE_URL/latest/FlatunChain-mac-arm64.dmg"
echo "    $BASE_URL/latest/FlatunChain-windows-x64.exe"
echo "    $BASE_URL/latest/FlatunChain-linux-x86_64.AppImage"
echo "  Sürüm manifesti: $BASE_URL/latest.json"
echo
echo "  Önerilen: git tag v$VERSION && git push origin v$VERSION"
