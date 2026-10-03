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

func TestApplyEnvironOverrides(t *testing.T) {
	cfg, err := Parse(bufio.NewScanner(strings.NewReader(sampleConf)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	applied, err := cfg.ApplyEnviron([]string{
		"s4=16",                      // names are case-insensitive
		"JMIN=70",                    // numeric override
		"UPSTREAM=198.51.100.7:1234", // endpoint override
		"I1=<r 100>",                 // CPS spec override
		"PATH=/usr/bin:/bin",         // unrelated variable, ignored
	})
	if err != nil {
		t.Fatalf("ApplyEnviron: %v", err)
	}
	if applied != 4 {
		t.Fatalf("applied = %d, want 4", applied)
	}
	if cfg.S4 != 16 {
		t.Fatalf("S4 = %d, want 16", cfg.S4)
	}
	if cfg.Jmin != 70 {
		t.Fatalf("Jmin = %d, want 70", cfg.Jmin)
	}
	if cfg.Endpoint != "198.51.100.7:1234" {
		t.Fatalf("Endpoint = %q", cfg.Endpoint)
	}
	if cfg.I[0] != "<r 100>" {
		t.Fatalf("I1 = %q", cfg.I[0])
	}
	// Untouched fields keep their file values.
	if cfg.S1 != 129 || cfg.H1.Lo != 1 {
		t.Fatalf("non-overridden fields changed: %+v", cfg)
	}
}

func TestApplyEnvironBadValueNamesVariable(t *testing.T) {
	cfg := &Config{}
	if _, err := cfg.ApplyEnviron([]string{"S1=notanumber"}); err == nil || !strings.Contains(err.Error(), "S1") {
		t.Fatalf("err = %v, want it to mention S1", err)
	}
}

// TestEnvOnlyMatchesFile checks that a config built purely from environment
// variables yields exactly the same obfuscation parameters as the equivalent
// awg0.conf file.
func TestEnvOnlyMatchesFile(t *testing.T) {
	fileCfg, err := Parse(bufio.NewScanner(strings.NewReader(sampleConf)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	envCfg := &Config{}
	if _, err := envCfg.ApplyEnviron([]string{
		"S1=129", "S2=106", "S3=23", "S4=12",
		"H1=1", "H2=2", "H3=3", "H4=4",
		"JC=3", "JMIN=56", "JMAX=188",
		"I1=<r 226>",
		"HPK=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=",
	}); err != nil {
		t.Fatalf("ApplyEnviron: %v", err)
	}

	fp, err := fileCfg.AWGParams()
	if err != nil {
		t.Fatalf("file AWGParams: %v", err)
	}
	ep, err := envCfg.AWGParams()
	if err != nil {
		t.Fatalf("env AWGParams: %v", err)
	}

	if fp.S1 != ep.S1 || fp.S2 != ep.S2 || fp.S3 != ep.S3 || fp.S4 != ep.S4 ||
		fp.Jc != ep.Jc || fp.Jmin != ep.Jmin || fp.Jmax != ep.Jmax ||
		fp.H1 != ep.H1 || fp.H2 != ep.H2 || fp.H3 != ep.H3 || fp.H4 != ep.H4 ||
		fp.HasHeaderProtection != ep.HasHeaderProtection ||
		fp.HeaderProtectionKey != ep.HeaderProtectionKey {
		t.Fatalf("env params differ from file params:\nfile=%+v\nenv =%+v", fp, ep)
	}
	if fp.I[0] == nil || ep.I[0] == nil || len(fp.I[0].Build()) != len(ep.I[0].Build()) {
		t.Fatalf("I1 chain mismatch: %v vs %v", fp.I[0], ep.I[0])
	}
}
