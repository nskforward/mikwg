package config

import (
	"bufio"
	"strings"
	"testing"
)

const sampleConf = `
[Interface]
PrivateKey = aGVsbG8gd29ybGQgdGhpcyBpcyBub3QgYSByZWFsIGtleSE=
Address = 10.8.2.5/32
DNS = 1.1.1.1, 8.8.8.8
MTU = 1408
Jc = 3
Jmin = 56
Jmax = 188
S1 = 129
S2 = 106
S3 = 23
S4 = 12
H1 = 1
H2 = 2
H3 = 3
H4 = 4
I1 = <r 226>
HeaderProtectionKey = AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=
ContentPaddingAddition = 18-36
RekeyAfterTime = 115-141
RekeyTimeout = 5-8
RejectAfterTime = 187-251
KeepaliveTimeout = 10-14
MaxHandshakeAttempts = 22-34

# AmneziaWG-3.1 - client
[Peer]
PublicKey = Hx8eHRwbGhkYFxYVFBMSERAPDg0MCwoJCAcGBQQDAgE=
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = 203.0.113.10:51822
`

func TestParseAndParams(t *testing.T) {
	cfg, err := Parse(bufio.NewScanner(strings.NewReader(sampleConf)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.S1 != 129 || cfg.S2 != 106 || cfg.S3 != 23 || cfg.S4 != 12 {
		t.Fatalf("S parsed wrong: %+v", cfg)
	}
	if cfg.H1.Lo != 1 || cfg.H1.Hi != 1 {
		t.Fatalf("H1 parsed wrong: %+v", cfg.H1)
	}
	if cfg.Jc != 3 || cfg.Jmin != 56 || cfg.Jmax != 188 {
		t.Fatalf("junk parsed wrong")
	}
	if cfg.I[0] != "<r 226>" {
		t.Fatalf("I1 parsed wrong: %q", cfg.I[0])
	}
	if cfg.Endpoint != "203.0.113.10:51822" {
		t.Fatalf("endpoint parsed wrong: %q", cfg.Endpoint)
	}

	params, err := cfg.AWGParams()
	if err != nil {
		t.Fatalf("AWGParams: %v", err)
	}
	if !params.HasHeaderProtection {
		t.Fatal("expected header protection enabled")
	}
	if params.I[0] == nil {
		t.Fatal("I1 chain not built")
	}
	if got := len(params.I[0].Build()); got != 226 {
		t.Fatalf("I1 length = %d, want 226", got)
	}
}

func TestParseRange(t *testing.T) {
	r, err := parseRange("115-141")
	if err != nil || r.Lo != 115 || r.Hi != 141 {
		t.Fatalf("parseRange: %v %+v", err, r)
	}
	if _, err := parseRange("141-115"); err == nil {
		t.Fatal("expected error for hi<lo")
	}
}
