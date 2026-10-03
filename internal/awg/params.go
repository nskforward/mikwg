// Package awg implements the AmneziaWG 3.x wire-format transformations on top of
// canonical WireGuard messages. It is deliberately cryptographic-free: it only
// adds/removes obfuscation layers (padding, header identifiers, header cipher,
// decoy packets). WireGuard's own AEAD/MAC protection is left untouched.
//
// See docs/protocol-notes.md for the specification and sources.
package awg

import (
	"errors"
	"fmt"
	mrand "math/rand/v2"

	"golang.org/x/crypto/chacha20"
)

// WireGuard message types.
const (
	MsgUnknown    = 0
	MsgInitiation = 1
	MsgResponse   = 2
	MsgCookie     = 3
	MsgTransport  = 4
)

// Canonical WireGuard message sizes.
const (
	SizeInitiation      = 148
	SizeResponse        = 92
	SizeCookie          = 64
	TransportHeaderSize = 16
	SizeTransport       = 32 // 16-byte header + 16-byte Poly1305 tag
	HeaderNonceSize     = 12
)

// Range is an inclusive uint32 range used for H1..H4 and the 3.x timer params.
type Range struct {
	Lo, Hi uint32
}

func (r Range) Contains(v uint32) bool { return r.Lo <= v && v <= r.Hi }

func (r Range) IsZero() bool { return r.Lo == 0 && r.Hi == 0 }

// Pick returns a random value from the inclusive range.
func (r Range) Pick() uint32 {
	if r.Hi <= r.Lo {
		return r.Lo
	}
	return r.Lo + mrand.Uint32N(r.Hi-r.Lo+1)
}

func (r Range) String() string {
	if r.Lo == r.Hi {
		return fmt.Sprintf("%d", r.Lo)
	}
	return fmt.Sprintf("%d-%d", r.Lo, r.Hi)
}

// Params holds every obfuscation parameter needed to (de)obfuscate traffic.
type Params struct {
	S1, S2, S3, S4 int
	H1, H2, H3, H4 Range

	Jc         int
	Jmin, Jmax int

	// I1..I5 concealment ("CPS") packets; nil entries are not sent.
	I [5]*ObfChain

	HeaderProtectionKey [32]byte
	HasHeaderProtection bool
}

var errShortPacket = errors.New("packet too short")

// Validate checks invariants that would otherwise produce broken traffic.
func (p *Params) Validate() error {
	if p.HasHeaderProtection {
		for i, s := range []int{p.S1, p.S2, p.S3, p.S4} {
			if s < HeaderNonceSize {
				return fmt.Errorf("S%d=%d must be >= %d when HeaderProtectionKey is set", i+1, s, HeaderNonceSize)
			}
		}
	}
	for i, s := range []int{p.S1, p.S2, p.S3, p.S4} {
		if s < 0 {
			return fmt.Errorf("S%d must be >= 0", i+1)
		}
	}
	if p.Jc < 0 || p.Jmin < 0 || p.Jmax < 0 {
		return errors.New("junk parameters must be >= 0")
	}
	if p.Jmax < p.Jmin {
		return errors.New("Jmax must be >= Jmin")
	}
	return nil
}

// headerCipher builds the ChaCha20 header-protection cipher for a datagram whose
// first 12 bytes are the nonce (the start of the S padding).
func (p *Params) headerCipher(salt []byte) (*chacha20.Cipher, error) {
	if !p.HasHeaderProtection {
		return nil, nil
	}
	if len(salt) < HeaderNonceSize {
		return nil, errShortPacket
	}
	return chacha20.NewUnauthenticatedCipher(p.HeaderProtectionKey[:], salt[:HeaderNonceSize])
}
