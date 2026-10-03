package awg

import (
	"crypto/rand"
	"testing"
)

func benchCanonical(msgType byte, size int) []byte {
	b := make([]byte, size)
	b[0] = msgType
	_, _ = rand.Read(b[4:])
	return b
}

// BenchmarkTransformOutTransport measures the outbound hot path as it existed
// before the headroom/in-place fast path was introduced (generic TransformOut).
func BenchmarkTransformOutTransport(b *testing.B) {
	p := testParams(b)
	in := benchCanonical(MsgTransport, 1400)
	b.ReportAllocs()
	b.SetBytes(int64(len(in)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		outs, err := p.TransformOut(in)
		if err != nil || len(outs) != 1 {
			b.Fatalf("TransformOut: %v (n=%d)", err, len(outs))
		}
	}
}

// BenchmarkTransformInTransport measures the inbound hot path. Each iteration
// copies a freshly wrapped datagram into a scratch buffer (as the proxy's read
// loop does) before transforming it in place.
func BenchmarkTransformInTransport(b *testing.B) {
	p := testParams(b)
	wrapped, err := p.wrapTransport(benchCanonical(MsgTransport, 1400))
	if err != nil {
		b.Fatal(err)
	}
	scratch := make([]byte, len(wrapped))
	b.ReportAllocs()
	b.SetBytes(int64(len(wrapped)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy(scratch, wrapped)
		if _, ok := p.TransformIn(scratch); !ok {
			b.Fatal("TransformIn rejected transport")
		}
	}
}

// BenchmarkWrapTransportInPlace measures the outbound hot path used by the
// proxy: the canonical message resides at buf[S4:] and is framed in place with
// no body copy and no allocation.
func BenchmarkWrapTransportInPlace(b *testing.B) {
	p := testParams(b)
	const n = 1400
	buf := make([]byte, p.S4+n)
	copy(buf[p.S4:], benchCanonical(MsgTransport, n))
	b.ReportAllocs()
	b.SetBytes(n)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.WrapTransportInPlace(buf, n); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTransformInInitiation measures the (cold) handshake path.
func BenchmarkTransformInInitiation(b *testing.B) {
	p := testParams(b)
	outs, err := p.TransformOut(benchCanonical(MsgInitiation, SizeInitiation))
	if err != nil {
		b.Fatal(err)
	}
	wrapped := outs[len(outs)-1]
	scratch := make([]byte, len(wrapped))
	b.ReportAllocs()
	b.SetBytes(int64(len(wrapped)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy(scratch, wrapped)
		if _, ok := p.TransformIn(scratch); !ok {
			b.Fatal("TransformIn rejected initiation")
		}
	}
}
