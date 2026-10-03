// Package proxy relays UDP between the router's WireGuard endpoint and the
// AmneziaWG server, applying the awg transformations in both directions.
package proxy

import (
	"context"
	"log"
	"net"
	"net/netip"
	"sync/atomic"
	"time"

	mrand "math/rand/v2"

	"github.com/nskforward/mikwg/internal/awg"
)

const maxDatagram = 65535

// Stats holds simple counters for diagnostics.
type Stats struct {
	ToUpstreamPackets   atomic.Uint64
	ToUpstreamBytes     atomic.Uint64
	ToRouterPackets     atomic.Uint64
	ToRouterBytes       atomic.Uint64
	Decoys              atomic.Uint64
	FromRouterDropped   atomic.Uint64
	FromUpstreamDropped atomic.Uint64
}

// Proxy is a 1:1 UDP transformer.
type Proxy struct {
	params   *awg.Params
	listen   string
	upstream string
	jitter   bool
	verbose  bool

	stats  Stats
	router atomic.Pointer[netip.AddrPort]
	local  atomic.Pointer[net.UDPAddr]
	ready  chan struct{}
}

// New creates a proxy. listen is the local address the router sends to;
// upstream is the AmneziaWG server endpoint (ip:port).
func New(params *awg.Params, listen, upstream string, jitter, verbose bool) *Proxy {
	return &Proxy{params: params, listen: listen, upstream: upstream, jitter: jitter, verbose: verbose, ready: make(chan struct{})}
}

// LocalAddr blocks until the listener is bound and returns its address.
func (p *Proxy) LocalAddr() *net.UDPAddr {
	<-p.ready
	return p.local.Load()
}

// Run blocks until ctx is cancelled.
func (p *Proxy) Run(ctx context.Context) error {
	laddr, err := net.ResolveUDPAddr("udp", p.listen)
	if err != nil {
		return err
	}
	uaddr, err := net.ResolveUDPAddr("udp", p.upstream)
	if err != nil {
		return err
	}

	l, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return err
	}
	defer l.Close()
	_ = l.SetReadBuffer(1 << 20)
	p.local.Store(l.LocalAddr().(*net.UDPAddr))
	close(p.ready)

	up, err := net.DialUDP("udp", nil, uaddr)
	if err != nil {
		return err
	}
	defer up.Close()
	_ = up.SetReadBuffer(1 << 20)

	// Close sockets when the context is cancelled so the readers unblock.
	go func() {
		<-ctx.Done()
		l.Close()
		up.Close()
	}()
	go p.report(ctx)

	go p.upstreamToRouter(ctx, up, l)
	p.routerToUpstream(ctx, l, up)
	return nil
}

func (p *Proxy) routerToUpstream(ctx context.Context, l, up *net.UDPConn) {
	// Leave S4 bytes of headroom so a transport datagram can be framed in
	// place: the kernel WireGuard message is read into buf[S4:] and the padding
	// prefix is written into buf[:S4] without moving the body.
	s4 := p.params.S4
	buf := make([]byte, s4+maxDatagram)
	for {
		n, ap, err := l.ReadFromUDPAddrPort(buf[s4:])
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		p.setRouter(ap)
		pkt := buf[s4 : s4+n]

		// Fast path: the overwhelming majority of packets are transport data.
		if n > 0 && pkt[0] == awg.MsgTransport {
			out, err := p.params.WrapTransportInPlace(buf, n)
			if err != nil {
				p.stats.FromRouterDropped.Add(1)
				if p.verbose {
					log.Printf("drop outbound: %v", err)
				}
				continue
			}
			if _, err := up.Write(out); err != nil {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			p.stats.ToUpstreamPackets.Add(1)
			p.stats.ToUpstreamBytes.Add(uint64(len(out)))
			continue
		}

		outs, err := p.params.TransformOut(pkt)
		if err != nil {
			p.stats.FromRouterDropped.Add(1)
			if p.verbose {
				log.Printf("drop outbound: %v", err)
			}
			continue
		}
		if len(outs) > 1 {
			p.stats.Decoys.Add(uint64(len(outs) - 1))
		}
		if p.jitter && pkt[0] == awg.MsgInitiation && len(outs) > 1 {
			time.Sleep(time.Duration(mrand.IntN(15)) * time.Millisecond)
		}
		for _, o := range outs {
			if _, err := up.Write(o); err != nil {
				if ctx.Err() != nil {
					return
				}
				break
			}
			p.stats.ToUpstreamPackets.Add(1)
			p.stats.ToUpstreamBytes.Add(uint64(len(o)))
		}
	}
}

func (p *Proxy) upstreamToRouter(ctx context.Context, up, l *net.UDPConn) {
	buf := make([]byte, maxDatagram)
	for {
		n, err := up.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		canonical, ok := p.params.TransformIn(buf[:n])
		if !ok {
			p.stats.FromUpstreamDropped.Add(1)
			if p.verbose {
				log.Printf("drop inbound: unrecognized %d-byte datagram", n)
			}
			continue
		}
		ap := p.router.Load()
		if ap == nil {
			continue
		}
		if _, err := l.WriteToUDPAddrPort(canonical, *ap); err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		p.stats.ToRouterPackets.Add(1)
		p.stats.ToRouterBytes.Add(uint64(len(canonical)))
	}
}

// setRouter records the last source address seen from the router (roaming-safe)
// without allocating on every packet: it only stores a new pointer when the
// address actually changes.
func (p *Proxy) setRouter(ap netip.AddrPort) {
	if cur := p.router.Load(); cur != nil && *cur == ap {
		return
	}
	p.router.Store(&ap)
}

func (p *Proxy) report(ctx context.Context) {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			log.Printf("stats: ->upstream %d pkt/%d B, ->router %d pkt/%d B, decoys %d, dropped router/upstream %d/%d",
				p.stats.ToUpstreamPackets.Load(), p.stats.ToUpstreamBytes.Load(),
				p.stats.ToRouterPackets.Load(), p.stats.ToRouterBytes.Load(),
				p.stats.Decoys.Load(),
				p.stats.FromRouterDropped.Load(), p.stats.FromUpstreamDropped.Load())
		}
	}
}
