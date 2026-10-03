// Command awg-converter turns a stock WireGuard client (the router's kernel
// WireGuard) into an AmneziaWG 3.x client by transforming packets in both
// directions. It runs inside a RouterOS container.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/nskforward/mikwg/internal/config"
	"github.com/nskforward/mikwg/internal/proxy"
)

var version = "dev"

func main() {
	confPath := flag.String("conf", "/etc/awg/awg0.conf", "path to the AmneziaWG config file")
	listen := flag.String("listen", "0.0.0.0:51820", "local address the router's WireGuard peer points at")
	upstream := flag.String("upstream", "", "override upstream AmneziaWG server (ip:port); defaults to Endpoint in conf")
	jitter := flag.Bool("jitter", false, "add timing jitter to handshake initiations")
	verbose := flag.Bool("v", false, "verbose logging")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	cfg, err := config.ParseFile(*confPath)
	if err != nil {
		log.Fatalf("failed to parse %s: %v", *confPath, err)
	}
	params, err := cfg.AWGParams()
	if err != nil {
		log.Fatalf("invalid obfuscation parameters: %v", err)
	}

	up := *upstream
	if up == "" {
		up = cfg.Endpoint
	}
	if up == "" {
		log.Fatal("no upstream endpoint: set Endpoint in conf or pass -upstream")
	}

	log.Printf("mikwg awg-converter %s", version)
	log.Printf("obfuscation: S=%d/%d/%d/%d H=%s/%s/%s/%s Jc=%d Jmin..Jmax=%d..%d headerProtection=%v",
		params.S1, params.S2, params.S3, params.S4,
		params.H1, params.H2, params.H3, params.H4,
		params.Jc, params.Jmin, params.Jmax, params.HasHeaderProtection)
	log.Printf("listen=%s upstream=%s", *listen, up)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	p := proxy.New(params, *listen, up, *jitter, *verbose)
	if err := p.Run(ctx); err != nil {
		log.Printf("proxy stopped: %v", err)
		os.Exit(1)
	}
}
