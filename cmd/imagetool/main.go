// Command imagetool builds a minimal linux/arm64 image (the format RouterOS
// /container expects) from a static binary and optionally publishes it to a
// container registry (e.g. Docker Hub) for use with /container/add
// remote-image=. It does not need a Docker daemon.
//
// The image has a single UNCOMPRESSED rootfs layer. RouterOS 7.24 accepts
// remote-image pulls of such a layer, and this is the same artifact shape that
// was verified on the router through the file= import path (see README.md).
package main

import (
	"archive/tar"
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

func main() {
	binary := flag.String("binary", "bin/awg-converter", "path to a static linux binary")
	out := flag.String("out", "", "output docker-archive tar (empty = do not write a tar)")
	image := flag.String("image", "docker.io/nskforward/mikwg", "image repository reference, without a tag")
	version := flag.String("version", "dev", "primary image tag (overridden by -tag)")
	tag := flag.String("tag", "", "primary image tag (default: value of -version)")
	tags := flag.String("tags", "", "additional comma-separated tags to push (e.g. \"latest\")")
	push := flag.Bool("push", false, "push the image to the registry")
	arch := flag.String("arch", "arm64", "image architecture")
	entrypoint := flag.String("entrypoint", "/awg-converter", "container entrypoint")
	flag.Parse()

	primary := *tag
	if primary == "" {
		primary = *version
	}
	if *out == "" && !*push {
		log.Fatal("nothing to do: set -out and/or -push")
	}

	data, err := os.ReadFile(*binary)
	if err != nil {
		log.Fatalf("read binary: %v", err)
	}

	img, err := buildImage(data, *arch, *entrypoint, primary)
	if err != nil {
		log.Fatalf("build image: %v", err)
	}

	// Registry the image belongs to, without a tag.
	repoRef, err := name.NewRepository(*image)
	if err != nil {
		log.Fatalf("parse image %q: %v", *image, err)
	}

	if *out != "" {
		ref, err := name.NewTag(fmt.Sprintf("%s:%s", repoRef.Name(), primary))
		if err != nil {
			log.Fatalf("ref: %v", err)
		}
		if err := tarball.WriteToFile(*out, ref, img); err != nil {
			log.Fatalf("write tar: %v", err)
		}
		log.Printf("wrote %s (arch=%s, entrypoint=%s, tag=%s)", *out, *arch, *entrypoint, primary)
	}

	if *push {
		opts := []remote.Option{remote.WithAuthFromKeychain(authn.DefaultKeychain)}
		if user, pass := os.Getenv("DOCKER_USERNAME"), os.Getenv("DOCKER_PASSWORD"); user != "" && pass != "" {
			opts = []remote.Option{remote.WithAuth(&authn.Basic{Username: user, Password: pass})}
		}

		all := append([]string{primary}, splitTags(*tags)...)
		for _, t := range all {
			tagRef, err := name.NewTag(fmt.Sprintf("%s:%s", repoRef.Name(), t))
			if err != nil {
				log.Fatalf("parse tag %q: %v", t, err)
			}
			if err := remote.Write(tagRef, img, opts...); err != nil {
				log.Fatalf("push %s: %v", tagRef.Name(), err)
			}
			log.Printf("pushed %s (arch=%s, tag=%s)", tagRef.Name(), *arch, t)
		}
	}
}

// buildImage assembles a single-layer image containing just the binary. The
// layer is uncompressed on purpose: RouterOS' container importer cannot read
// gzip-compressed layers ("error getting layer file / failed to load next
// entry"), and the remote-image path is expected to be equally strict.
func buildImage(data []byte, arch, entrypoint, version string) (v1.Image, error) {
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
		return nil, err
	}
	if _, err := tw.Write(data); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}

	layer := static.NewLayer(buf.Bytes(), types.OCIUncompressedLayer)

	img, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		return nil, err
	}
	img, err = mutate.Config(img, v1.Config{
		Entrypoint: []string{entrypoint},
		User:       "65534:65534",
		WorkingDir: "/",
		Labels: map[string]string{
			"org.opencontainers.image.title":       "mikwg awg-converter",
			"org.opencontainers.image.version":     version,
			"org.opencontainers.image.source":      "https://github.com/nskforward/mikwg",
			"org.opencontainers.image.licenses":    "GPL-3.0-only",
			"org.opencontainers.image.description": "AmneziaWG 3.1 packet converter for RouterOS",
		},
	})
	if err != nil {
		return nil, err
	}
	cf, err := img.ConfigFile()
	if err != nil {
		return nil, err
	}
	cf = cf.DeepCopy()
	cf.Architecture = arch
	cf.OS = "linux"
	cf.Created = v1.Time{Time: time.Unix(0, 0)}
	return mutate.ConfigFile(img, cf)
}

func splitTags(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}
