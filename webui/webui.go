// Package webui, cüzdan web arayüzünü binary'nin İÇİNE gömer.
//
// Arayüz dosyası derleme sırasında binary'ye katıldığı için uygulama hangi
// dizinden, nasıl çalıştırılırsa çalıştırılsın arayüz her zaman açılır —
// dağıtılan tek dosyanın yanında hiçbir ek dosya gerekmez.
package webui

import "embed"

//go:embed templates/index.html
var files embed.FS

// IndexHTML, gömülü cüzdan arayüzünü döndürür
func IndexHTML() ([]byte, error) {
	return files.ReadFile("templates/index.html")
}
