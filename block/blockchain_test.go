package block

import (
	"testing"

	"blockchain/utils"
	"blockchain/wallet"
)

func TestBlockReward(t *testing.T) {
	cases := []struct {
		height int
		want   int64
	}{
		{0, 0}, // genesis ödülsüz
		{1, 50 * utils.UNITS_PER_FLATUN},
		{HALVING_INTERVAL - 1, 50 * utils.UNITS_PER_FLATUN},
		{HALVING_INTERVAL, 25 * utils.UNITS_PER_FLATUN},
		{2 * HALVING_INTERVAL, 12*utils.UNITS_PER_FLATUN + 50_000_000}, // 12.5
		{6 * HALVING_INTERVAL, TAIL_REWARD},                            // 50/64 < 1 -> kuyruk
		{100 * HALVING_INTERVAL, TAIL_REWARD},
	}
	for _, c := range cases {
		if got := BlockReward(c.height); got != c.want {
			t.Errorf("BlockReward(%d)=%d, beklenen %d", c.height, got, c.want)
		}
	}
}

// minedChain: A'nın miner olduğu, n blok kazılmış test zinciri
func minedChain(t *testing.T, minerAddress string, blocks int) *Blockchain {
	t.Helper()
	bc := NewBlockchain(minerAddress, 0)
	for i := 0; i < blocks; i++ {
		if !bc.Mining() {
			t.Fatal("mining başarısız")
		}
	}
	return bc
}

func TestMiningRewardAndBalance(t *testing.T) {
	a := wallet.NewWallet()
	bc := minedChain(t, a.BlockchainAddress(), 3)

	want := 3 * 50 * utils.UNITS_PER_FLATUN
	if got := bc.CalculateTotalAmount(a.BlockchainAddress()); got != want {
		t.Errorf("miner bakiyesi=%d, beklenen %d", got, want)
	}
}

func TestAddTransactionFlow(t *testing.T) {
	a := wallet.NewWallet()
	b := wallet.NewWallet()
	bc := minedChain(t, a.BlockchainAddress(), 2) // A: 100 FLATUN

	tenFlatun := 10 * utils.UNITS_PER_FLATUN
	tx := wallet.NewTransaction(a.PrivateKey(), a.PublicKey(), a.BlockchainAddress(), b.BlockchainAddress(), tenFlatun)
	sig := tx.GenerateSignature()

	// Geçerli işlem kabul edilmeli
	if !bc.AddTransaction(a.BlockchainAddress(), b.BlockchainAddress(), tenFlatun, 0, tx.Timestamp(), a.PublicKey(), sig) {
		t.Fatal("geçerli işlem reddedildi")
	}

	// Replay: aynı imzalı işlem ikinci kez reddedilmeli
	if bc.AddTransaction(a.BlockchainAddress(), b.BlockchainAddress(), tenFlatun, 0, tx.Timestamp(), a.PublicKey(), sig) {
		t.Fatal("replay işlemi kabul edildi")
	}

	// Adres bağlama: B'nin pubkey'i ile A adresinden harcama reddedilmeli
	tx2 := wallet.NewTransaction(b.PrivateKey(), b.PublicKey(), a.BlockchainAddress(), b.BlockchainAddress(), tenFlatun)
	sig2 := tx2.GenerateSignature()
	if bc.AddTransaction(a.BlockchainAddress(), b.BlockchainAddress(), tenFlatun, 0, tx2.Timestamp(), b.PublicKey(), sig2) {
		t.Fatal("adres-pubkey uyumsuz işlem kabul edildi (kritik güvenlik açığı)")
	}

	// Bakiye: B'nin parası yok, gönderemez
	tx3 := wallet.NewTransaction(b.PrivateKey(), b.PublicKey(), b.BlockchainAddress(), a.BlockchainAddress(), tenFlatun)
	sig3 := tx3.GenerateSignature()
	if bc.AddTransaction(b.BlockchainAddress(), a.BlockchainAddress(), tenFlatun, 0, tx3.Timestamp(), b.PublicKey(), sig3) {
		t.Fatal("bakiyesiz işlem kabul edildi")
	}

	// Havuz-içi double-spend: A kalan bakiyesinin tamamını + fazlasını isteyemez
	remaining := bc.CalculateTotalAmount(a.BlockchainAddress()) - tenFlatun // havuzdaki bekleyen düşülür
	tx4 := wallet.NewTransaction(a.PrivateKey(), a.PublicKey(), a.BlockchainAddress(), b.BlockchainAddress(), remaining+1)
	sig4 := tx4.GenerateSignature()
	if bc.AddTransaction(a.BlockchainAddress(), b.BlockchainAddress(), remaining+1, 0, tx4.Timestamp(), a.PublicKey(), sig4) {
		t.Fatal("havuz-içi double-spend kabul edildi")
	}

	// Negatif tutar reddedilmeli
	if bc.AddTransaction(a.BlockchainAddress(), b.BlockchainAddress(), -5, 0, tx.Timestamp(), a.PublicKey(), sig) {
		t.Fatal("negatif tutar kabul edildi")
	}
}

func TestValidChainFullValidation(t *testing.T) {
	a := wallet.NewWallet()
	b := wallet.NewWallet()
	bc := minedChain(t, a.BlockchainAddress(), 2)

	// İçinde gerçek bir transfer olan blok kaz
	tenFlatun := 10 * utils.UNITS_PER_FLATUN
	tx := wallet.NewTransaction(a.PrivateKey(), a.PublicKey(), a.BlockchainAddress(), b.BlockchainAddress(), tenFlatun)
	if !bc.AddTransaction(a.BlockchainAddress(), b.BlockchainAddress(), tenFlatun, 0, tx.Timestamp(), a.PublicKey(), tx.GenerateSignature()) {
		t.Fatal("işlem eklenemedi")
	}
	bc.Mining()

	// Sağlıklı zincir geçerli olmalı
	if !bc.ValidChain(bc.Chain()) {
		t.Fatal("geçerli zincir reddedildi")
	}

	// Coinbase miktarını kurcala -> geçersiz olmalı
	chain := bc.Chain()
	for _, tr := range chain[1].transactions {
		if tr.senderBlockchainAddress == MINING_SENDER {
			tr.value += 1 // 1 alt birim fazla ödül
		}
	}
	if bc.ValidChain(chain) {
		t.Fatal("kurcalanmış coinbase kabul edildi")
	}
}

func TestConsiderChain(t *testing.T) {
	a := wallet.NewWallet()
	bc1 := minedChain(t, a.BlockchainAddress(), 3) // 4 blok (genesis + 3)
	bc2 := NewBlockchain(wallet.NewWallet().BlockchainAddress(), 0)

	// Daha fazla iş içeren zincir benimsenmeli (itme yolu: POST /submit)
	if !bc2.ConsiderChain(bc1.Chain()) {
		t.Fatal("daha güçlü zincir benimsenmedi")
	}
	if len(bc2.Chain()) != len(bc1.Chain()) {
		t.Fatalf("zincir uzunluğu %d, beklenen %d", len(bc2.Chain()), len(bc1.Chain()))
	}

	// Aynı zincir ikinci kez benimsenmemeli (daha iyi değil)
	if bc2.ConsiderChain(bc1.Chain()) {
		t.Fatal("özdeş zincir yeniden benimsendi")
	}

	// Daha zayıf zincir reddedilmeli
	bc3 := NewBlockchain(wallet.NewWallet().BlockchainAddress(), 0)
	if bc1.ConsiderChain(bc3.Chain()) {
		t.Fatal("daha zayıf zincir benimsendi")
	}
}

func TestSigningPayloadMatchesWallet(t *testing.T) {
	// wallet.Transaction ve block.Transaction imza yükleri birebir aynı olmalı;
	// bu test ayrıştıklarında ilk kırılan yer olsun
	a := wallet.NewWallet()
	b := wallet.NewWallet()
	wtx := wallet.NewTransaction(a.PrivateKey(), a.PublicKey(), a.BlockchainAddress(), b.BlockchainAddress(), 12345)
	btx := NewSignedTransaction(a.BlockchainAddress(), b.BlockchainAddress(), 12345, 0, wtx.Timestamp(), "", "")

	sig := wtx.GenerateSignature()
	bc := NewBlockchain(a.BlockchainAddress(), 0)
	if !bc.VerifyTransactionSignature(a.PublicKey(), sig, btx) {
		t.Fatal("cüzdan imzası node tarafında doğrulanamadı — imza yükleri ayrışmış")
	}
}
