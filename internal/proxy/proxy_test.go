package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"net"
	"testing"
	"time"

	"github.com/nskforward/mikwg/internal/awg"
)

func testParams(t *testing.T) *awg.Params {
	t.Helper()
	i1, err := awg.ParseObfChain("<r 40>")
	if err != nil {
		t.Fatal(err)
	}
	p := &awg.Params{
		S1: 12, S2: 12, S3: 12, S4: 12,
		H1: awg.Range{Lo: 1, Hi: 1}, H2: awg.Range{Lo: 2, Hi: 2}, H3: awg.Range{Lo: 3, Hi: 3}, H4: awg.Range{Lo: 4, Hi: 4},
		Jc: 2, Jmin: 20, Jmax: 40,
		I:                   [5]*awg.ObfChain{i1},
		HasHeaderProtection: true,
	}
	if _, err := rand.Read(p.HeaderProtectionKey[:]); err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestProxyRoundTrip drives the full UDP path: a fake router sends a canonical
// handshake initiation and a fake server replies with a wrapped transport
// packet. The proxy must transform in both directions.
func TestProxyRoundTrip(t *testing.T) {
	params := testParams(t)

	srv, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	p := New(params, "127.0.0.1:0", srv.LocalAddr().String(), false, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = p.Run(ctx) }()
	listenAddr := p.LocalAddr()
	if listenAddr == nil {
		t.Fatal("proxy did not bind")
	}

	cl, err := net.DialUDP("udp", nil, listenAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	// 1. Router sends a canonical initiation; the proxy must prepend decoys.
	init := make([]byte, awg.SizeInitiation)
	init[0] = awg.MsgInitiation
	for i := 4; i < len(init); i++ {
		init[i] = byte(i)
	}
	if _, err := cl.Write(init); err != nil {
		t.Fatal(err)
	}

	_ = srv.SetReadDeadline(time.Now().Add(3 * time.Second))
	var recovered []byte
	var serverSeen int
	for serverSeen < 10 {
		buf := make([]byte, 4096)
		n, raddr, err := srv.ReadFromUDP(buf)
		if err != nil {
			t.Fatalf("server read: %v", err)
		}
		serverSeen++
		if canonical, ok := params.TransformIn(buf[:n]); ok {
			recovered = canonical
			// 2. Server replies with a wrapped transport packet.
			resp := make([]byte, 48)
			resp[0] = awg.MsgTransport
			for i := 1; i < len(resp); i++ {
				resp[i] = byte(255 - i)
			}
			outs, err := params.TransformOut(resp)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := srv.WriteToUDP(outs[0], raddr); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if serverSeen < 4 {
		t.Fatalf("expected decoys before initiation, saw only %d packets", serverSeen)
	}
	if !bytes.Equal(recovered, init) {
		t.Fatalf("upstream received wrong initiation")
	}

	// 3. Router must receive the canonical transport packet back.
	_ = cl.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 4096)
	n, err := cl.Read(buf)
	if err != nil {
		t.Fatalf("router read: %v", err)
	}
	if n < awg.SizeTransport || buf[0] != awg.MsgTransport {
		t.Fatalf("router got unexpected packet: type=%d len=%d", buf[0], n)
	}
}
