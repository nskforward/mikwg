package awg

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ObfChain builds a standalone concealment packet from an AmneziaWG CPS spec,
// e.g. "<r 226>" (226 random bytes). Only the outbound direction is needed:
// the peer treats these packets as decoys and never validates them.
type ObfChain struct {
	spec string
	obfs []obf
}

// ParseObfChain parses a CPS spec. An empty spec yields (nil, nil).
func ParseObfChain(spec string) (*ObfChain, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}

	var obfs []obf
	remaining := spec
	for {
		start := strings.IndexByte(remaining, '<')
		if start == -1 {
			break
		}
		end := strings.IndexByte(remaining[start:], '>')
		if end == -1 {
			return nil, errors.New("missing enclosing >")
		}
		end += start

		tag := remaining[start+1 : end]
		parts := strings.Fields(tag)
		if len(parts) == 0 {
			return nil, errors.New("empty tag")
		}
		val := ""
		if len(parts) > 1 {
			val = parts[1]
		}
		o, err := newObf(parts[0], val)
		if err != nil {
			return nil, fmt.Errorf("tag <%s>: %w", parts[0], err)
		}
		obfs = append(obfs, o)
		remaining = remaining[end+1:]
	}
	if len(obfs) == 0 {
		return nil, nil
	}
	return &ObfChain{spec: spec, obfs: obfs}, nil
}

// Spec returns the original spec string.
func (c *ObfChain) Spec() string { return c.spec }

// Build renders the packet. CPS specs are built with no source data.
func (c *ObfChain) Build() []byte {
	n := 0
	for _, o := range c.obfs {
		n += o.obfLen(0)
	}
	out := make([]byte, n)
	pos := 0
	for _, o := range c.obfs {
		pos += o.write(out[pos:])
	}
	return out
}

// obf is the outbound-only subset of AmneziaWG's obfuscator interface.
type obf interface {
	// obfLen is the output length for a source payload of srcLen bytes.
	obfLen(srcLen int) int
	// write renders the obfuscated bytes into dst and returns bytes written.
	write(dst []byte) int
}

func newObf(key, val string) (obf, error) {
	switch key {
	case "b":
		return newBytesObf(val)
	case "r":
		return newLenObf(val, randBytes)
	case "rc":
		return newLenObf(val, randChars)
	case "rd":
		return newLenObf(val, randDigits)
	case "t":
		return &timestampObf{}, nil
	case "d":
		return &dataObf{}, nil
	case "ds":
		return &dataStringObf{}, nil
	case "dz":
		return newDataSizeObf(val)
	default:
		return nil, errors.New("unknown tag")
	}
}

// --- <b HEX> ---

type bytesObf struct{ data []byte }

func newBytesObf(val string) (obf, error) {
	val = strings.TrimPrefix(val, "0x")
	if val == "" {
		return nil, errors.New("empty argument")
	}
	if len(val)%2 != 0 {
		return nil, errors.New("odd amount of symbols")
	}
	b, err := hex.DecodeString(val)
	if err != nil {
		return nil, err
	}
	return &bytesObf{data: b}, nil
}

func (o *bytesObf) obfLen(int) int { return len(o.data) }
func (o *bytesObf) write(dst []byte) int {
	return copy(dst, o.data)
}

// --- <r N> / <rc N> / <rd N> ---

type alphabetFunc func(dst []byte)

type lenObf struct {
	length int
	enc    alphabetFunc
}

func newLenObf(val string, enc alphabetFunc) (obf, error) {
	n, err := strconv.Atoi(val)
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, errors.New("negative length")
	}
	return &lenObf{length: n, enc: enc}, nil
}

func (o *lenObf) obfLen(int) int { return o.length }
func (o *lenObf) write(dst []byte) int {
	b := dst[:o.length]
	if _, err := rand.Read(b); err != nil {
		// crypto/rand should not fail; leave zeroes on failure.
		return o.length
	}
	if o.enc != nil {
		o.enc(b)
	}
	return o.length
}

func randBytes(dst []byte) {}

func randChars(dst []byte) {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for i := range dst {
		dst[i] = chars[int(dst[i])%len(chars)]
	}
}

func randDigits(dst []byte) {
	const digits = "0123456789"
	for i := range dst {
		dst[i] = digits[int(dst[i])%len(digits)]
	}
}

// --- <t> ---

type timestampObf struct{}

func (o *timestampObf) obfLen(int) int { return 4 }
func (o *timestampObf) write(dst []byte) int {
	binary.BigEndian.PutUint32(dst[:4], uint32(time.Now().Unix()))
	return 4
}

// --- <d> ---

type dataObf struct{}

func (o *dataObf) obfLen(srcLen int) int { return srcLen }
func (o *dataObf) write(dst []byte) int  { return 0 } // CPS has no source payload

// --- <ds> ---

type dataStringObf struct{}

func (o *dataStringObf) obfLen(srcLen int) int {
	return base64.RawStdEncoding.EncodedLen(srcLen)
}
func (o *dataStringObf) write(dst []byte) int { return 0 }

// --- <dz N> ---

type dataSizeObf struct{ length int }

func newDataSizeObf(val string) (obf, error) {
	n, err := strconv.Atoi(val)
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, errors.New("negative length")
	}
	return &dataSizeObf{length: n}, nil
}

func (o *dataSizeObf) obfLen(int) int { return o.length }
func (o *dataSizeObf) write(dst []byte) int {
	// CPS packets carry no source payload, so the encoded size is zero.
	for i := 0; i < o.length; i++ {
		dst[i] = 0
	}
	return o.length
}
