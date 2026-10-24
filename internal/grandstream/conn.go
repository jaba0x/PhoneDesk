package grandstream

import (
	"bytes"
	"context"
	"crypto/tls"
	"net"
)

// GXP16xx firmware 1.0.2.x answers api.values.post with the CGI script's own
// first line ahead of the HTTP headers:
//
//	HTTP/1.0 200 OK
//	#!/bin/ash
//	Cache-Control: no-cache, must-revalidate
//	Content-Type: application/json
//
// A browser skips a header line without a colon; Go's HTTP client refuses the
// whole answer ("malformed MIME header: missing colon"). headerFixConn sits
// under the HTTP client and drops such lines from the header block of the
// response. Connections aren't reused (DisableKeepAlives), so each one
// carries exactly one response and only its first header block is looked at.
type headerFixConn struct {
	net.Conn
	buf  []byte // header bytes collected so far
	out  []byte // fixed bytes waiting to be handed to the reader
	err  error  // error met while collecting, returned once out is drained
	done bool   // header block passed; everything else goes straight through
}

const maxHeaderBlock = 64 << 10

func (c *headerFixConn) Read(p []byte) (int, error) {
	if len(c.out) > 0 {
		n := copy(p, c.out)
		c.out = c.out[n:]
		return n, nil
	}
	if c.err != nil {
		err := c.err
		c.err = nil
		return 0, err
	}
	if c.done {
		return c.Conn.Read(p)
	}
	tmp := make([]byte, 8192)
	for {
		n, err := c.Conn.Read(tmp)
		c.buf = append(c.buf, tmp[:n]...)
		if end := headerEnd(c.buf); end >= 0 {
			c.out = append(fixHeaderBlock(c.buf[:end]), c.buf[end:]...)
			c.buf, c.done = nil, true
			c.err = err
			return c.Read(p)
		}
		if err != nil || len(c.buf) > maxHeaderBlock || (len(c.buf) >= 5 && !bytes.HasPrefix(c.buf, []byte("HTTP/"))) {
			// not something we understand, or the end came first: pass it on as is
			c.out, c.buf, c.done = c.buf, nil, true
			c.err = err
			if len(c.out) == 0 && err != nil {
				c.err = nil
				return 0, err
			}
			return c.Read(p)
		}
	}
}

// headerEnd returns the index just past the blank line that ends the header
// block, or -1 when it isn't there yet. Both \r\n\r\n and \n\n count.
func headerEnd(b []byte) int {
	end := -1
	if i := bytes.Index(b, []byte("\r\n\r\n")); i >= 0 {
		end = i + 4
	}
	if i := bytes.Index(b, []byte("\n\n")); i >= 0 && (end < 0 || i+2 < end) {
		end = i + 2
	}
	return end
}

// fixHeaderBlock rewrites a header block (status line through the blank
// line) keeping the status line and every line with a colon in it.
func fixHeaderBlock(b []byte) []byte {
	lines := bytes.Split(bytes.TrimRight(b, "\r\n"), []byte("\n"))
	out := make([]byte, 0, len(b))
	for i, l := range lines {
		l = bytes.TrimRight(l, "\r")
		if i > 0 && !bytes.Contains(l, []byte(":")) {
			continue
		}
		out = append(out, l...)
		out = append(out, '\r', '\n')
	}
	return append(out, '\r', '\n')
}

// dialFixed dials a plain connection and wraps it.
func dialFixed(d *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := d.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		return &headerFixConn{Conn: conn}, nil
	}
}

// dialTLSFixed dials, runs the TLS handshake with cfg and wraps the result,
// so the fix sees the decrypted stream.
func dialTLSFixed(d *net.Dialer, cfg *tls.Config) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		raw, err := d.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		host, _, _ := net.SplitHostPort(addr)
		tcfg := cfg.Clone()
		if tcfg.ServerName == "" {
			tcfg.ServerName = host
		}
		tc := tls.Client(raw, tcfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			raw.Close()
			return nil, err
		}
		return &headerFixConn{Conn: tc}, nil
	}
}
