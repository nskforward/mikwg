package awg

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func testParams(t testing.TB) *Params {
	t.Helper()
	i1, err := ParseObfChain("<r 226>")
	if err != nil {
		t.Fatalf("ParseObfChain: %v", err)
	}
	p := &Params{
		S1: 129, S2: 106, S3: 23, S4: 12,
		H1: Range{Lo: 1, Hi: 1}, H2: Range{Lo: 2, Hi: 2}, H3: Range{Lo: 3, Hi: 3}, H4: Range{Lo: 4, Hi: 4},
		Jc: 3, Jmin: 56, Jmax: 188,
		I:                   [5]*ObfChain{i1},
		HasHeaderProtection: true,
	}
	if _, err := rand.Read(p.HeaderProtectionKey[:]); err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return p
}

func canonical(msgType byte, size int) []byte {
	b := make([]byte, size)
	b[0] = msgType
	for i := 4; i < size; i++ {
		b[i] = byte(i)
	}
	return b
}

func TestRoundTripHandshakes(t *testing.T) {
	p := testParams(t)
	cases := []struct {
		name    string
		msgType byte
		size    int
	}{
		{"initiation", MsgInitiation, SizeInitiation},
		{"response", MsgResponse, SizeResponse},
		{"cookie", MsgCookie, SizeCookie},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := canonical(tc.msgType, tc.size)
			outs, err := p.TransformOut(in)
			if err != nil {
				t.Fatalf("TransformOut: %v", err)
			}
			wrapped := outs[len(outs)-1]
			got, ok := p.TransformIn(wrapped)
			if !ok {
				t.Fatalf("TransformIn rejected wrapped %s", tc.name)
			}
			if !bytes.Equal(got, in) {
				t.Fatalf("round-trip mismatch\n got %x\nwant %x", got, in)
			}
		})
	}
}

func TestRoundTripTransport(t *testing.T) {
	p := testParams(t)
	in := canonical(MsgTransport, 200)
	outs, err := p.TransformOut(in)
	if err != nil {
		t.Fatalf("TransformOut: %v", err)
	}
	if len(outs) != 1 {
		t.Fatalf("transport should be a single packet, got %d", len(outs))
	}
	got, ok := p.TransformIn(outs[0])
	if !ok {
		t.Fatal("TransformIn rejected transport")
	}
	if !bytes.Equal(got, in) {
		t.Fatalf("round-trip mismatch")
	}
}

// TestWrapTransportInPlace covers the proxy's outbound fast path: the canonical
// message sits at buf[S4:] and is framed in place, reusing the caller's buffer.
func TestWrapTransportInPlace(t *testing.T) {
	p := testParams(t)
	in := canonical(MsgTransport, 200)
	buf := make([]byte, p.S4+len(in))
	copy(buf[p.S4:], in)

	out, err := p.WrapTransportInPlace(buf, len(in))
	if err != nil {
		t.Fatalf("WrapTransportInPlace: %v", err)
	}
	if len(out) != p.S4+len(in) {
		t.Fatalf("datagram size %d, want %d", len(out), p.S4+len(in))
	}
	if &out[0] != &buf[0] {
		t.Fatal("WrapTransportInPlace must reuse the caller's buffer (zero-copy)")
	}
	got, ok := p.TransformIn(out)
	if !ok {
		t.Fatal("TransformIn rejected the in-place transport datagram")
	}
	if !bytes.Equal(got, in) {
		t.Fatalf("round-trip mismatch\n got %x\nwant %x", got, in)
	}

	if _, err := p.WrapTransportInPlace(make([]byte, p.S4+8), 8); err == nil {
		t.Fatal("short transport message must be rejected")
	}
}

func TestInitiationDecoys(t *testing.T) {
	p := testParams(t)
	in := canonical(MsgInitiation, SizeInitiation)
	outs, err := p.TransformOut(in)
	if err != nil {
		t.Fatal(err)
	}
	// I1 (configured) + Jc junk + initiation.
	if want := 1 + p.Jc + 1; len(outs) != want {
		t.Fatalf("got %d packets, want %d", len(outs), want)
	}
	if len(outs[0]) != 226 {
		t.Fatalf("I1 should be 226 bytes, got %d", len(outs[0]))
	}
	for i := 1; i <= p.Jc; i++ {
		n := len(outs[i])
		if n < p.Jmin || n >= p.Jmax {
			t.Fatalf("junk %d size %d outside [%d,%d)", i, n, p.Jmin, p.Jmax)
		}
	}
	last := outs[len(outs)-1]
	if len(last) != p.S1+SizeInitiation {
		t.Fatalf("initiation size %d, want %d", len(last), p.S1+SizeInitiation)
	}
}

func TestTransportLayout(t *testing.T) {
	p := testParams(t)
	in := canonical(MsgTransport, 100)
	outs, _ := p.TransformOut(in)
	if len(outs[0]) != p.S4+len(in) {
		t.Fatalf("transport size %d, want %d", len(outs[0]), p.S4+len(in))
	}
}

func TestTransformInRejectsJunk(t *testing.T) {
	p := testParams(t)
	junk := make([]byte, 400)
	if _, err := rand.Read(junk); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.TransformIn(junk); ok {
		t.Fatal("random junk must not be recognized")
	}
}

func TestNoHeaderProtection(t *testing.T) {
	p := testParams(t)
	p.HasHeaderProtection = false
	p.HeaderProtectionKey = [32]byte{}
	// S values below 12 are allowed again without HP.
	p.S1, p.S2, p.S3, p.S4 = 5, 5, 5, 5
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	for _, tc := range []struct {
		mt   byte
		size int
	}{
		{MsgInitiation, SizeInitiation},
		{MsgResponse, SizeResponse},
		{MsgCookie, SizeCookie},
		{MsgTransport, 64},
	} {
		in := canonical(tc.mt, tc.size)
		outs, err := p.TransformOut(in)
		if err != nil {
			t.Fatalf("TransformOut type %d: %v", tc.mt, err)
		}
		got, ok := p.TransformIn(outs[len(outs)-1])
		if !ok || !bytes.Equal(got, in) {
			t.Fatalf("round-trip failed for type %d", tc.mt)
		}
	}
}

func TestValidateHPRules(t *testing.T) {
	p := testParams(t)
	p.S4 = 8
	if err := p.Validate(); err == nil {
		t.Fatal("expected validation error for S4 < 12 with HP enabled")
	}
}
