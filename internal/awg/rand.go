package awg

import (
	"crypto/rand"
	"sync"
)

// randPoolSize is the size of the buffered crypto/rand chunk. Outbound padding
// (the S1..S4 prefix, i.e. the header-protection nonce) is drawn from here in
// bulk so a getrandom syscall is amortized over many packets instead of being
// paid per packet. The bytes stay cryptographically strong.
const randPoolSize = 4096

type randBuf struct {
	mu  sync.Mutex
	buf [randPoolSize]byte
	off int
}

func (r *randBuf) fill(dst []byte) {
	if len(dst) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for len(dst) > 0 {
		if r.off >= len(r.buf) {
			if _, err := rand.Read(r.buf[:]); err != nil {
				// crypto/rand does not fail in practice; fall back to a direct
				// read so callers still get random bytes.
				_, _ = rand.Read(dst)
				return
			}
			r.off = 0
		}
		n := copy(dst, r.buf[r.off:])
		r.off += n
		dst = dst[n:]
	}
}

// rng is process-wide: the converter has a single writer per direction, so the
// uncontended lock is cheap and no per-Params state is needed.
var rng randBuf

// randFill fills dst with cryptographically strong random bytes, reading from a
// small process-wide buffer to avoid a syscall per packet.
func randFill(dst []byte) { rng.fill(dst) }
