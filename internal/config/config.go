// Package config parses an AmneziaWG (awg0.conf) file and turns it into the
// obfuscation parameters used by the converter. The WireGuard private key is
// never used by the converter; it stays on the router.
package config

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/nskforward/mikwg/internal/awg"
)

// Config is a parsed AmneziaWG client configuration.
type Config struct {
	// [Interface]
	PrivateKey             string
	Address                string
	DNS                    string
	MTU                    int
	Jc                     int
	Jmin, Jmax             int
	S1, S2, S3, S4         int
	H1, H2, H3, H4         awg.Range
	I                      [5]string
	HeaderProtectionKey    string
	ContentPaddingAddition string
	RekeyAfterTime         string
	RekeyTimeout           string
	RejectAfterTime        string
	KeepaliveTimeout       string
	MaxHandshakeAttempts   string

	// [Peer]
	PeerPublicKey string
	Endpoint      string
	AllowedIPs    string
}

// ParseFile reads and parses an awg0.conf file.
func ParseFile(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(bufio.NewScanner(f))
}

type lineScanner interface {
	Scan() bool
	Text() string
}

// Parse parses an INI-style awg0.conf from a scanner.
func Parse(s lineScanner) (*Config, error) {
	c := &Config{}
	section := ""
	line := 0
	for s.Scan() {
		line++
		raw := strings.TrimSpace(s.Text())
		if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, ";") {
			continue
		}
		if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
			section = strings.ToLower(strings.Trim(raw, "[]"))
			continue
		}
		eq := strings.IndexByte(raw, '=')
		if eq < 0 {
			return nil, fmt.Errorf("line %d: missing '='", line)
		}
		key := strings.ToLower(strings.TrimSpace(raw[:eq]))
		val := strings.TrimSpace(raw[eq+1:])
		if err := c.set(section, key, val); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
	}
	return c, nil
}

func (c *Config) set(section, key, val string) error {
	switch section {
	case "interface":
		return c.setInterface(key, val)
	case "peer":
		return c.setPeer(key, val)
	default:
		return nil // ignore unknown sections
	}
}

func (c *Config) setInterface(key, val string) error {
	var err error
	switch key {
	case "privatekey":
		c.PrivateKey = val
	case "address":
		c.Address = val
	case "dns":
		c.DNS = val
	case "mtu":
		c.MTU, err = atoi(val)
	case "jc":
		c.Jc, err = atoi(val)
	case "jmin":
		c.Jmin, err = atoi(val)
	case "jmax":
		c.Jmax, err = atoi(val)
	case "s1":
		c.S1, err = atoi(val)
	case "s2":
		c.S2, err = atoi(val)
	case "s3":
		c.S3, err = atoi(val)
	case "s4":
		c.S4, err = atoi(val)
	case "h1":
		c.H1, err = parseRange(val)
	case "h2":
		c.H2, err = parseRange(val)
	case "h3":
		c.H3, err = parseRange(val)
	case "h4":
		c.H4, err = parseRange(val)
	case "i1":
		c.I[0] = val
	case "i2":
		c.I[1] = val
	case "i3":
		c.I[2] = val
	case "i4":
		c.I[3] = val
	case "i5":
		c.I[4] = val
	case "headerprotectionkey":
		c.HeaderProtectionKey = val
	case "contentpaddingaddition":
		c.ContentPaddingAddition = val
	case "rekeyaftertime":
		c.RekeyAfterTime = val
	case "rekeytimeout":
		c.RekeyTimeout = val
	case "rejectaftertime":
		c.RejectAfterTime = val
	case "keepalivetimeout":
		c.KeepaliveTimeout = val
	case "maxhandshakeattempts":
		c.MaxHandshakeAttempts = val
	}
	return err
}

func (c *Config) setPeer(key, val string) error {
	switch key {
	case "publickey":
		c.PeerPublicKey = val
	case "endpoint":
		c.Endpoint = val
	case "allowedips":
		c.AllowedIPs = val
	}
	return nil
}

// AWGParams builds the obfuscation parameter set.
func (c *Config) AWGParams() (*awg.Params, error) {
	p := &awg.Params{
		S1: c.S1, S2: c.S2, S3: c.S3, S4: c.S4,
		H1: c.H1, H2: c.H2, H3: c.H3, H4: c.H4,
		Jc: c.Jc, Jmin: c.Jmin, Jmax: c.Jmax,
	}
	if c.HeaderProtectionKey != "" {
		key, err := decodeKey(c.HeaderProtectionKey)
		if err != nil {
			return nil, fmt.Errorf("HeaderProtectionKey: %w", err)
		}
		copy(p.HeaderProtectionKey[:], key)
		p.HasHeaderProtection = true
	}
	for i, spec := range c.I {
		chain, err := awg.ParseObfChain(spec)
		if err != nil {
			return nil, fmt.Errorf("I%d: %w", i+1, err)
		}
		p.I[i] = chain
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func decodeKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}

func atoi(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q", s)
	}
	return n, nil
}

func parseRange(s string) (awg.Range, error) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, "-", 2)
	lo, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil {
		return awg.Range{}, fmt.Errorf("invalid range %q", s)
	}
	hi := lo
	if len(parts) == 2 {
		hi, err = strconv.ParseUint(parts[1], 10, 32)
		if err != nil {
			return awg.Range{}, fmt.Errorf("invalid range %q", s)
		}
	}
	if hi < lo {
		return awg.Range{}, fmt.Errorf("range %q: hi < lo", s)
	}
	return awg.Range{Lo: uint32(lo), Hi: uint32(hi)}, nil
}
