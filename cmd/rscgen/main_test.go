package main

import (
	"strings"
	"testing"
	"text/template"

	"github.com/nskforward/mikwg/internal/config"
)

func renderScript(t *testing.T, d data) string {
	t.Helper()
	tmpl := template.Must(template.New("rsc").Parse(script))
	var b strings.Builder
	if err := tmpl.Execute(&b, d); err != nil {
		t.Fatalf("execute template: %v", err)
	}
	return b.String()
}

func TestTemplateEnvMode(t *testing.T) {
	d := data{
		Container: "awg-converter",
		EnvMode:   true,
		EnvList:   "awg-env",
		Envs: []envEntry{
			{Key: "UPSTREAM", Value: "203.0.113.10:51822"},
			{Key: "S4", Value: "12"},
			{Key: "I1", Value: "<r 226>"},
		},
		ServerIP: "203.0.113.10",
		WGName:   "awg",
		AddrList: "to_vpn_list",
		RTTable:  "to_vpn_table",
		ConnMark: "to_vpn_mark",
	}
	out := renderScript(t, d)

	for _, want := range []string{
		`:do { /container/envs/remove [find where list="awg-env"] } on-error={}`,
		`/container/envs/add list=awg-env key="S4" value="12"`,
		`/container/envs/add list=awg-env key="I1" value="<r 226>"`,
		`envlists=awg-env`,
		`mountlists=""`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("env-mode script missing %q", want)
		}
	}
	if strings.Contains(out, "mountlists=awg-cfg") {
		t.Fatal("env-mode script must not attach the awg0.conf mount")
	}
	if !strings.Contains(out, "key=\"S4\"") || !strings.Contains(out, "container/restart") {
		t.Fatal("env-mode script should include an in-place edit hint")
	}
}

func TestTemplateLegacyMountMode(t *testing.T) {
	d := data{
		Container:    "awg-converter",
		EnvMode:      false,
		ContainerTar: "awg-converter-arm64.tar",
		ServerIP:     "203.0.113.10",
		WGName:       "awg",
		AddrList:     "to_vpn_list",
		RTTable:      "to_vpn_table",
		ConnMark:     "to_vpn_mark",
	}
	out := renderScript(t, d)

	if !strings.Contains(out, "mountlists=awg-cfg") {
		t.Fatal("legacy mode should attach the awg0.conf mount")
	}
	if !strings.Contains(out, `/container/mounts/add list=awg-cfg`) {
		t.Fatal("legacy mode should create the config mount")
	}
	if strings.Contains(out, "/container/envs/add") {
		t.Fatal("legacy mode should not generate environment variables")
	}
}

func TestTemplateUpdateScript(t *testing.T) {
	// Registry mode: the one-command updater is installed and targets the tag
	// baked into this generation.
	d := data{
		Container: "awg-converter",
		Image:     "nskforward/mikwg:latest",
		EnvMode:   true,
		EnvList:   "awg-env",
		ServerIP:  "203.0.113.10",
		WGName:    "awg",
		AddrList:  "to_vpn_list",
		RTTable:   "to_vpn_table",
		ConnMark:  "to_vpn_mark",
	}
	out := renderScript(t, d)
	for _, want := range []string{
		`:do { /system/script/remove [find name="awg-update"] } on-error={}`,
		`/system/script/add name=awg-update source={`,
		`/container/update awg-converter`,
		`remote-image=nskforward/mikwg:latest`,
		`[find name="awg-watchdog"] disabled=yes`,
		`[find name="awg-watchdog"] disabled=no`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("registry-mode script missing %q", want)
		}
	}

	// Offline (tar) mode: no updater body, and any stale one is removed.
	tar := data{
		Container:    "awg-converter",
		EnvMode:      true,
		EnvList:      "awg-env",
		ContainerTar: "awg-converter-arm64.tar",
		ServerIP:     "203.0.113.10",
		WGName:       "awg",
		AddrList:     "to_vpn_list",
		RTTable:      "to_vpn_table",
		ConnMark:     "to_vpn_mark",
	}
	tarOut := renderScript(t, tar)
	if !strings.Contains(tarOut, `:do { /system/script/remove [find name="awg-update"] } on-error={}`) {
		t.Fatal("tar mode should remove a stale awg-update script")
	}
	if strings.Contains(tarOut, `name=awg-update source={`) {
		t.Fatal("tar mode must not install the remote-image updater")
	}
}

func TestBuildEnvs(t *testing.T) {
	cfg := &config.Config{
		S1: 129, S2: 106, S3: 23, S4: 12,
		Jc: 3, Jmin: 56, Jmax: 188,
		I:                   [5]string{"<r 226>", "", "", "", ""},
		HeaderProtectionKey: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=",
	}
	got := map[string]string{}
	for _, e := range buildEnvs(cfg, "203.0.113.10:51822") {
		got[e.Key] = e.Value
	}
	want := map[string]string{
		"UPSTREAM": "203.0.113.10:51822",
		"S1":       "129",
		"S2":       "106",
		"S3":       "23",
		"S4":       "12",
		"H1":       "0",
		"H2":       "0",
		"H3":       "0",
		"H4":       "0",
		"JC":       "3",
		"JMIN":     "56",
		"JMAX":     "188",
		"HPK":      "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=",
		"I1":       "<r 226>",
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %q, want %q", k, got[k], v)
		}
	}
	if _, ok := got["I2"]; ok {
		t.Fatal("empty I values must be omitted")
	}
	if len(got) != len(want) {
		t.Fatalf("got %d env entries, want %d: %v", len(got), len(want), got)
	}
}

func TestRscQuote(t *testing.T) {
	if got := rscQuote(`a"b$c\d`); got != `a\"b\$c\\d` {
		t.Fatalf("rscQuote = %q", got)
	}
}
