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
	"strings"
	"syscall"

	"github.com/nskforward/mikwg/internal/config"
	"github.com/nskforward/mikwg/internal/proxy"
)

var version = "dev"

func main() {
	confPath := flag.String("conf", "/etc/awg/awg0.conf", "path to the AmneziaWG config file; a missing file is ignored (parameters then come from the environment)")
	listen := flag.String("listen", "0.0.0.0:51820", "local address the router's WireGuard peer points at (env LISTEN)")
	upstream := flag.String("upstream", "", "override upstream AmneziaWG server (ip:port); defaults to Endpoint/UPSTREAM")
	jitter := flag.Bool("jitter", false, "add timing jitter to handshake initiations (env JITTER)")
	verbose := flag.Bool("v", false, "verbose logging (env VERBOSE)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })

	cfg := &config.Config{}
	confSource := "env"
	if *confPath != "" {
		if _, err := os.Stat(*confPath); err == nil {
			parsed, err := config.ParseFile(*confPath)
			if err != nil {
				log.Fatalf("failed to parse %s: %v", *confPath, err)
			}
			cfg = parsed
			confSource = "file"
		} else if set["conf"] {
			log.Fatalf("failed to read %s: %v", *confPath, err)
		} else {
			log.Printf("no config file at %s; expecting environment variables", *confPath)
		}
	}
	applied, err := cfg.ApplyEnviron(os.Environ())
	if err != nil {
		log.Fatalf("invalid environment config: %v", err)
	}
	if applied > 0 {
		if confSource == "file" {
			confSource = "file+env"
		} else {
			confSource = "env"
		}
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
		log.Fatal("no upstream endpoint: set UPSTREAM, Endpoint in conf, or pass -upstream")
	}

	localListen := *listen
	if !set["listen"] {
		if v := os.Getenv("LISTEN"); v != "" {
			localListen = v
		}
	}
	useJitter := *jitter
	if !set["jitter"] {
		useJitter = useJitter || envBool("JITTER")
	}
	useVerbose := *verbose
	if !set["v"] {
		useVerbose = useVerbose || envBool("VERBOSE")
	}

	log.Printf("mikwg awg-converter %s", version)
	log.Printf("config source: %s", confSource)
	log.Printf("obfuscation: S=%d/%d/%d/%d H=%s/%s/%s/%s Jc=%d Jmin..Jmax=%d..%d headerProtection=%v",
		params.S1, params.S2, params.S3, params.S4,
		params.H1, params.H2, params.H3, params.H4,
		params.Jc, params.Jmin, params.Jmax, params.HasHeaderProtection)
	log.Printf("listen=%s upstream=%s", localListen, up)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	p := proxy.New(params, localListen, up, useJitter, useVerbose)
	if err := p.Run(ctx); err != nil {
		log.Printf("proxy stopped: %v", err)
		os.Exit(1)
	}
}

// envBool reports whether an environment variable is set to a truthy value.
func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
