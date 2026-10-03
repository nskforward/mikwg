// Command imagetool builds a minimal linux/arm64 docker-archive tar (the format
// RouterOS /container/add file= expects) from a static binary, without needing a
// Docker daemon. Useful on machines without a running Docker engine.
package main

import (
	"archive/tar"
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

func main() {
	binary := flag.String("binary", "bin/awg-converter", "path to a static linux binary")
	out := flag.String("out", "awg-converter-arm64.tar", "output docker-archive tar")
	version := flag.String("version", "dev", "image version tag")
	arch := flag.String("arch", "arm64", "image architecture")
	entrypoint := flag.String("entrypoint", "/awg-converter", "container entrypoint")
	flag.Parse()

	data, err := os.ReadFile(*binary)
	if err != nil {
		log.Fatalf("read binary: %v", err)
	}

	// Build a single-layer rootfs containing just the binary.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name:     "awg-converter",
		Mode:     0o755,
		Size:     int64(len(data)),
		Typeflag: tar.TypeReg,
		ModTime:  time.Unix(0, 0),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		log.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		log.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		log.Fatal(err)
	}

	// RouterOS' container importer cannot read gzip-compressed layers: it
	// expects a legacy docker-archive with an uncompressed rootfs tar, otherwise
	// import fails with "error getting layer file / failed to load next entry".
	// Use an uncompressed OCI layer so the resulting archive is importable.
	layer := static.NewLayer(buf.Bytes(), types.OCIUncompressedLayer)

	img, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		log.Fatalf("append: %v", err)
	}
	img, err = mutate.Config(img, v1.Config{
		Entrypoint: []string{*entrypoint},
		User:       "65534:65534",
		WorkingDir: "/",
	})
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	cf, err := img.ConfigFile()
	if err != nil {
		log.Fatal(err)
	}
	cf = cf.DeepCopy()
	cf.Architecture = *arch
	cf.OS = "linux"
	cf.Created = v1.Time{Time: time.Unix(0, 0)}
	img, err = mutate.ConfigFile(img, cf)
	if err != nil {
		log.Fatalf("configfile: %v", err)
	}

	ref, err := name.ParseReference(fmt.Sprintf("mikwg/awg-converter:%s", *version))
	if err != nil {
		log.Fatalf("ref: %v", err)
	}
	if err := tarball.WriteToFile(*out, ref, img); err != nil {
		log.Fatalf("write tar: %v", err)
	}
	log.Printf("wrote %s (arch=%s, entrypoint=%s)", *out, *arch, *entrypoint)
}
