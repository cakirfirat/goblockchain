package utils

import "testing"

func TestParseFLATUN(t *testing.T) {
	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"1", 100_000_000, false},
		{"1.5", 150_000_000, false},
		{"0.00000001", 1, false},
		{"50", 5_000_000_000, false},
		{"210000000", 21_000_000_000_000_000, false}, // toplam arz sınırı
		{"0", 0, false},
		{"1.", 100_000_000, false},
		{".5", 50_000_000, false},
		{"", 0, true},
		{"-1", 0, true},
		{"1.123456789", 0, true}, // 9 ondalık hane — fazla
		{"abc", 0, true},
	}
	for _, c := range cases {
		got, err := ParseFLATUN(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseFLATUN(%q) hata=%v, beklenen hata=%v", c.in, err, c.wantErr)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("ParseFLATUN(%q)=%d, beklenen %d", c.in, got, c.want)
		}
	}
}

func TestFormatFLATUN(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{100_000_000, "1"},
		{150_000_000, "1.5"},
		{1, "0.00000001"},
		{5_000_000_000, "50"},
		{0, "0"},
		{-150_000_000, "-1.5"},
	}
	for _, c := range cases {
		if got := FormatFLATUN(c.in); got != c.want {
			t.Errorf("FormatFLATUN(%d)=%q, beklenen %q", c.in, got, c.want)
		}
	}
}

func TestParseFormatRoundTrip(t *testing.T) {
	for _, s := range []string{"1", "1.5", "0.00000001", "123.45678901"} {
		units, err := ParseFLATUN(s)
		if err != nil {
			t.Fatalf("ParseFLATUN(%q): %v", s, err)
		}
		if got := FormatFLATUN(units); got != s {
			t.Errorf("gidiş-dönüş %q -> %d -> %q", s, units, got)
		}
	}
}
