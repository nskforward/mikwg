package awg

import (
	"encoding/binary"
	"fmt"
	mrand "math/rand/v2"
)

// TransformOut converts a canonical WireGuard packet coming from the kernel
// into one or more AmneziaWG datagrams to send upstream. Only handshake
// initiations are preceded by decoy packets (CPS I1..I5, then junk Jc), matching
// amneziawg-go's send order.
func (p *Params) TransformOut(pkt []byte) ([][]byte, error) {
	if len(pkt) < 4 {
		return nil, errShortPacket
	}
	switch pkt[0] {
	case MsgInitiation:
		body, err := p.wrapHandshake(pkt, p.S1, p.H1, SizeInitiation)
		if err != nil {
			return nil, err
		}
		out := p.decoys()
		return append(out, body), nil
	case MsgResponse:
		body, err := p.wrapHandshake(pkt, p.S2, p.H2, SizeResponse)
		if err != nil {
			return nil, err
		}
		return [][]byte{body}, nil
	case MsgCookie:
		body, err := p.wrapHandshake(pkt, p.S3, p.H3, SizeCookie)
		if err != nil {
			return nil, err
		}
		return [][]byte{body}, nil
	case MsgTransport:
		body, err := p.wrapTransport(pkt)
		if err != nil {
			return nil, err
		}
		return [][]byte{body}, nil
	default:
		return nil, fmt.Errorf("unknown WireGuard message type %d", pkt[0])
	}
}

// TransformIn converts an AmneziaWG datagram received from the server back into
// a canonical WireGuard packet for the kernel. ok is false for decoys/unknown
// traffic, which must be dropped.
func (p *Params) TransformIn(pkt []byte) (canonical []byte, ok bool) {
	// Mirror amneziawg-go's DeterminePacketTypeAndPadding order.
	if out, ok := p.unwrapHandshake(pkt, p.S1, p.H1, SizeInitiation, MsgInitiation); ok {
		return out, true
	}
	if out, ok := p.unwrapHandshake(pkt, p.S2, p.H2, SizeResponse, MsgResponse); ok {
		return out, true
	}
	if out, ok := p.unwrapHandshake(pkt, p.S3, p.H3, SizeCookie, MsgCookie); ok {
		return out, true
	}
	if out, ok := p.unwrapTransport(pkt); ok {
		return out, true
	}
	return nil, false
}

// decoys builds the concealment packets sent before a handshake initiation.
func (p *Params) decoys() [][]byte {
	out := make([][]byte, 0, len(p.I)+p.Jc)
	for _, c := range p.I {
		if c != nil {
			out = append(out, c.Build())
		}
	}
	for i := 0; i < p.Jc; i++ {
		out = append(out, p.junkPacket())
	}
	return out
}

func (p *Params) junkPacket() []byte {
	n := p.Jmin
	if p.Jmax > p.Jmin {
		n += mrand.IntN(p.Jmax - p.Jmin)
	}
	b := make([]byte, n)
	randFill(b)
	return b
}

// wrapHandshake builds [pad(S)][HP(msg)] for a fixed-size handshake message.
func (p *Params) wrapHandshake(pkt []byte, s int, h Range, wantSize int) ([]byte, error) {
	if len(pkt) != wantSize {
		return nil, fmt.Errorf("bad handshake size %d, want %d", len(pkt), wantSize)
	}
	out := make([]byte, s+len(pkt))
	randFill(out[:s])
	copy(out[s:], pkt)
	binary.LittleEndian.PutUint32(out[s:s+4], h.Pick())
	if err := p.hpXOR(out[:s], out[s:]); err != nil {
		return nil, err
	}
	return out, nil
}

// wrapTransport builds [pad(S4)][HP(first 16 bytes)][ciphertext...] into a
// freshly allocated datagram. This is the cold path (tests and handshake-adjacent
// callers); the proxy's hot path uses WrapTransportInPlace.
func (p *Params) wrapTransport(pkt []byte) ([]byte, error) {
	if len(pkt) < SizeTransport {
		return nil, errShortPacket
	}
	out := make([]byte, p.S4+len(pkt))
	copy(out[p.S4:], pkt)
	return p.encodeTransport(out, len(pkt))
}

// WrapTransportInPlace frames the canonical transport message that already
// occupies buf[S4:S4+n], exactly as the proxy's read buffer is laid out: the S4
// random prefix and the protected 16-byte header are written in place and
// buf[:S4+n] is returned. The message body is never copied and nothing is
// allocated.
func (p *Params) WrapTransportInPlace(buf []byte, n int) ([]byte, error) {
	return p.encodeTransport(buf, n)
}

// encodeTransport frames the canonical message of msgLen bytes located at
// dst[S4:] and returns dst[:S4+msgLen].
func (p *Params) encodeTransport(dst []byte, msgLen int) ([]byte, error) {
	s := p.S4
	if msgLen < SizeTransport || len(dst) < s+msgLen {
		return nil, errShortPacket
	}
	randFill(dst[:s])
	out := dst[:s+msgLen]
	binary.LittleEndian.PutUint32(out[s:s+4], p.H4.Pick())

	cip, err := p.headerCipher(out[:s])
	if err != nil {
		return nil, err
	}
	if cip != nil {
		head := out[s : s+TransportHeaderSize]
		cip.XORKeyStream(head, head)
	}
	return out, nil
}

// hpXOR applies the header-protection keystream over an entire handshake message.
func (p *Params) hpXOR(salt, msg []byte) error {
	cip, err := p.headerCipher(salt)
	if err != nil {
		return err
	}
	if cip != nil {
		cip.XORKeyStream(msg, msg)
	}
	return nil
}

func (p *Params) unwrapHandshake(pkt []byte, s int, h Range, msgSize, msgType int) ([]byte, bool) {
	// Handshake datagrams have a fixed total size (S padding + fixed-size
	// message), so a length mismatch rules this type out without even touching
	// the cipher. This keeps ordinary transport packets off the handshake
	// probes, of which there would otherwise be three per packet.
	if len(pkt) != s+msgSize {
		return nil, false
	}
	cip, err := p.headerCipher(pkt)
	if err != nil {
		return nil, false
	}
	// The type field sits at offset s; verify it against the H range using the
	// first 4 keystream bytes (nonce = pkt[:12]).
	var ks4 [4]byte
	if cip != nil {
		cip.XORKeyStream(ks4[:], ks4[:])
	}
	got := binary.LittleEndian.Uint32(pkt[s:s+4]) ^ binary.LittleEndian.Uint32(ks4[:])
	if !h.Contains(got) {
		return nil, false
	}

	// Decrypt in place: on success the datagram storage is dead (the caller
	// sends the result and reuses the read buffer). Note the nonce lives in
	// pkt[:12], before s, so it is untouched. The first 4 bytes are replaced
	// with the canonical message type below.
	msg := pkt[s : s+msgSize]
	if cip != nil {
		cip.XORKeyStream(msg[4:], msg[4:])
	}
	binary.LittleEndian.PutUint32(msg[0:4], uint32(msgType))
	return msg, true
}

func (p *Params) unwrapTransport(pkt []byte) ([]byte, bool) {
	s := p.S4
	if len(pkt) < s+SizeTransport {
		return nil, false
	}
	cip, err := p.headerCipher(pkt)
	if err != nil {
		return nil, false
	}
	var ks4 [4]byte
	if cip != nil {
		cip.XORKeyStream(ks4[:], ks4[:])
	}
	got := binary.LittleEndian.Uint32(pkt[s:s+4]) ^ binary.LittleEndian.Uint32(ks4[:])
	if !p.H4.Contains(got) {
		return nil, false
	}

	// Decrypt the 16-byte header in place and restore the canonical type. The
	// ciphertext body from msg[TransportHeaderSize:] is left untouched; the
	// nonce at pkt[:S4] is before the message, so it is not clobbered.
	msg := pkt[s:]
	if cip != nil {
		cip.XORKeyStream(msg[4:TransportHeaderSize], msg[4:TransportHeaderSize])
	}
	binary.LittleEndian.PutUint32(msg[0:4], MsgTransport)
	return msg, true
}
