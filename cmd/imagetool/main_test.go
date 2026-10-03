package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

const payload = "binary-payload"

func TestBuildUncompressedImage(t *testing.T) {
	// The offline tar is imported with /container/add file=, which only accepts
	// an uncompressed rootfs layer.
	img, err := buildImage([]byte(payload), "arm64", "/awg-converter", "1.2.3", false)
	if err != nil {
		t.Fatalf("buildImage: %v", err)
	}
	assertConfig(t, img, "1.2.3")

	layers, err := img.Layers()
	if err != nil {
		t.Fatalf("layers: %v", err)
	}
	if len(layers) != 1 {
		t.Fatalf("layers = %d, want 1", len(layers))
	}
	if mt, _ := layers[0].MediaType(); mt != types.OCIUncompressedLayer {
		t.Errorf("layer media type = %s, want %s", mt, types.OCIUncompressedLayer)
	}

	rc, err := layers[0].Compressed()
	if err != nil {
		t.Fatalf("layer: %v", err)
	}
	raw := mustRead(t, rc)
	assertTarContains(t, raw, "awg-converter", payload)
}

func TestBuildAndPushCompressedImage(t *testing.T) {
	// The registry push (remote-image) is decompressed by RouterOS on pull, so
	// the layer must be a gzip-compressed Docker layer.
	img, err := buildImage([]byte(payload), "arm64", "/awg-converter", "1.2.3", true)
	if err != nil {
		t.Fatalf("buildImage: %v", err)
	}
	assertConfig(t, img, "1.2.3")

	layers, err := img.Layers()
	if err != nil {
		t.Fatalf("layers: %v", err)
	}
	if len(layers) != 1 {
		t.Fatalf("layers = %d, want 1", len(layers))
	}
	if mt, _ := layers[0].MediaType(); mt != types.DockerLayer {
		t.Errorf("layer media type = %s, want %s", mt, types.DockerLayer)
	}

	grc, err := layers[0].Compressed()
	if err != nil {
		t.Fatalf("layer: %v", err)
	}
	raw := mustRead(t, grc)
	if len(raw) < 2 || raw[0] != 0x1f || raw[1] != 0x8b {
		t.Fatalf("layer is not gzip compressed (len=%d, magic=% x)", len(raw), raw)
	}
	gr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	untarred, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	assertTarContains(t, untarred, "awg-converter", payload)

	// Push to an in-process OCI registry (no Docker daemon) — the same write
	// path used against Docker Hub — and read it back.
	s := httptest.NewServer(registry.New())
	defer s.Close()
	host := strings.TrimPrefix(s.URL, "http://")
	ref, err := name.NewTag(host+"/nskforward/mikwg:1.2.3", name.Insecure)
	if err != nil {
		t.Fatalf("ref: %v", err)
	}
	if err := remote.Write(ref, img, remote.WithAuth(authn.Anonymous)); err != nil {
		t.Fatalf("push: %v", err)
	}
	got, err := remote.Image(ref, remote.WithAuth(authn.Anonymous))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	assertConfig(t, got, "1.2.3")
}

func assertConfig(t *testing.T, img v1.Image, version string) {
	t.Helper()
	cf, err := img.ConfigFile()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if cf.OS != "linux" || cf.Architecture != "arm64" {
		t.Errorf("platform = %s/%s, want linux/arm64", cf.OS, cf.Architecture)
	}
	if got := cf.Config.Entrypoint; len(got) != 1 || got[0] != "/awg-converter" {
		t.Errorf("entrypoint = %v", got)
	}
	if cf.Config.User != "65534:65534" {
		t.Errorf("user = %q", cf.Config.User)
	}
	if cf.Config.Labels["org.opencontainers.image.version"] != version {
		t.Errorf("version label = %q", cf.Config.Labels["org.opencontainers.image.version"])
	}
	if _, ok := cf.Config.Labels["org.opencontainers.image.source"]; !ok {
		t.Error("source label missing")
	}
}

func mustRead(t *testing.T, rc io.ReadCloser) []byte {
	t.Helper()
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read layer: %v", err)
	}
	return b
}

func assertTarContains(t *testing.T, raw []byte, name, content string) {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(raw))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar: %v", err)
		}
		if hdr.Name == name {
			b, _ := io.ReadAll(tr)
			if string(b) != content {
				t.Errorf("%s content = %q, want %q", name, b, content)
			}
			return
		}
	}
	t.Errorf("tar does not contain %q", name)
}
