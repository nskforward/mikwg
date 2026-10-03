package main

import (
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

// TestBuildAndPush exercises the exact artifact shape published to the
// registry: a single uncompressed rootfs layer, the right entrypoint/user/arch
// and OCI labels. It pushes to an in-process OCI registry (no Docker daemon),
// which is the same write path used against Docker Hub.
func TestBuildAndPush(t *testing.T) {
	const version = "1.2.3"
	img, err := buildImage([]byte("binary-payload"), "arm64", "/awg-converter", version)
	if err != nil {
		t.Fatalf("buildImage: %v", err)
	}

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

	layers, err := img.Layers()
	if err != nil {
		t.Fatalf("layers: %v", err)
	}
	if len(layers) != 1 {
		t.Fatalf("layers = %d, want 1", len(layers))
	}
	if mt, _ := layers[0].MediaType(); mt != types.OCIUncompressedLayer {
		t.Errorf("layer media type = %s, want %s (RouterOS cannot read compressed layers)", mt, types.OCIUncompressedLayer)
	}

	s := httptest.NewServer(registry.New())
	defer s.Close()
	host := strings.TrimPrefix(s.URL, "http://")
	ref, err := name.NewTag(host+"/nskforward/mikwg:"+version, name.Insecure)
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
	gotcf, err := got.ConfigFile()
	if err != nil {
		t.Fatalf("read back config: %v", err)
	}
	if gotcf.Architecture != "arm64" || gotcf.Config.Entrypoint[0] != "/awg-converter" {
		t.Errorf("round-tripped image mismatch: %+v", gotcf)
	}
	if _, ok := gotcf.Config.Labels["org.opencontainers.image.source"]; !ok {
		t.Error("source label missing after round trip")
	}

	var _ v1.Image = got
}
