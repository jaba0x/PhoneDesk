// Package relay copies firmware images over SSH to a server the phones can
// reach, for networks where the phones can't connect back to this computer
// (zero trust access, one-way VPNs). The phones then fetch from that server.
package relay

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Options describe the SSH server and how to log in to it.
type Options struct {
	Host     string
	Port     int // 0 = 22
	User     string
	Password string // optional when KeyFile is set
	KeyFile  string // optional private key file (OpenSSH format)
	// KnownKey is the server's host key fingerprint from an earlier
	// connection ("SHA256:…"). Empty learns it; a different key is refused.
	KnownKey string
	Timeout  time.Duration
}

var ErrHostKeyMismatch = errors.New("the relay server's host key changed")

// Client is one SSH connection.
type Client struct {
	c           *ssh.Client
	Fingerprint string
}

// Dial connects and authenticates.
func Dial(ctx context.Context, o Options) (*Client, error) {
	if strings.TrimSpace(o.Host) == "" {
		return nil, errors.New("no relay server set")
	}
	if o.User == "" {
		return nil, errors.New("no SSH user for the relay server")
	}
	port := o.Port
	if port == 0 {
		port = 22
	}
	timeout := o.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	var auth []ssh.AuthMethod
	if o.KeyFile != "" {
		keyFile := o.KeyFile
		if strings.HasPrefix(keyFile, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				keyFile = home + keyFile[1:]
			}
		}
		pem, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, fmt.Errorf("reading the key file: %w", err)
		}
		var signer ssh.Signer
		if o.Password != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(pem, []byte(o.Password))
			if err != nil {
				signer, err = ssh.ParsePrivateKey(pem)
			}
		} else {
			signer, err = ssh.ParsePrivateKey(pem)
		}
		if err != nil {
			return nil, fmt.Errorf("the key file can't be read: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if o.Password != "" {
		pw := o.Password
		auth = append(auth, ssh.Password(pw), ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
			out := make([]string, len(questions))
			for i := range out {
				out[i] = pw
			}
			return out, nil
		}))
	}
	if len(auth) == 0 {
		return nil, errors.New("set a password or a key file for the relay server")
	}
	cl := &Client{}
	cfg := &ssh.ClientConfig{
		User:    o.User,
		Auth:    auth,
		Timeout: timeout,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			cl.Fingerprint = ssh.FingerprintSHA256(key)
			if o.KnownKey != "" && o.KnownKey != cl.Fingerprint {
				return ErrHostKeyMismatch
			}
			return nil
		},
	}
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(o.Host, strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("can't reach %s: %w", o.Host, shortErr(err))
	}
	sc, chans, reqs, err := ssh.NewClientConn(conn, o.Host, cfg)
	if err != nil {
		conn.Close()
		if errors.Is(err, ErrHostKeyMismatch) || strings.Contains(err.Error(), ErrHostKeyMismatch.Error()) {
			return nil, ErrHostKeyMismatch
		}
		if strings.Contains(err.Error(), "unable to authenticate") {
			return nil, errors.New("SSH login refused: check the user, password or key")
		}
		return nil, err
	}
	cl.c = ssh.NewClient(sc, chans, reqs)
	return cl, nil
}

func (c *Client) Close() error { return c.c.Close() }

func shortErr(err error) error {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i >= 0 {
		return errors.New(s[i+2:])
	}
	return err
}

// run executes one command and returns its output.
func (c *Client) run(cmd string, stdin io.Reader) (string, error) {
	s, err := c.c.NewSession()
	if err != nil {
		return "", err
	}
	defer s.Close()
	var out, errb bytes.Buffer
	s.Stdout, s.Stderr, s.Stdin = &out, &errb, stdin
	if err := s.Run(cmd); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), errors.New(msg)
	}
	return out.String(), nil
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Check makes sure dir exists and is writable, and reports the server.
func (c *Client) Check(dir string) (string, error) {
	if dir == "" {
		return "", errors.New("no remote folder set")
	}
	probe := path.Join(dir, ".phonedesk-write-test")
	out, err := c.run("mkdir -p "+quote(dir)+" && touch "+quote(probe)+" && rm -f "+quote(probe)+" && uname -n && df -h "+quote(dir)+" | tail -1 | awk '{print $4}'", nil)
	if err != nil {
		return "", fmt.Errorf("%s isn't writable for this user: %s", dir, err.Error())
	}
	f := strings.Fields(out)
	if len(f) >= 2 {
		return fmt.Sprintf("%s, %s free in %s", f[0], f[1], dir), nil
	}
	return strings.TrimSpace(out), nil
}

// RemoteSize returns the size of a remote file, or -1 when it doesn't exist.
func (c *Client) RemoteSize(file string) (int64, error) {
	out, err := c.run("if [ -f "+quote(file)+" ]; then wc -c < "+quote(file)+"; else echo -1; fi", nil)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("odd answer from the server: %q", strings.TrimSpace(out))
	}
	return n, nil
}

// Upload copies a local file to dir/name on the server, unless a file of
// the same size and checksum is already there. It reports whether it copied.
func (c *Client) Upload(ctx context.Context, local, dir, name string) (bool, error) {
	f, err := os.Open(local)
	if err != nil {
		return false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false, err
	}
	dst := path.Join(dir, name)
	if n, err := c.RemoteSize(dst); err == nil && n == st.Size() {
		if same, _ := c.sameChecksum(f, dst); same {
			return false, nil
		}
		f.Seek(0, io.SeekStart)
	}
	tmp := dst + ".part"
	cmd := "mkdir -p " + quote(dir) + " && cat > " + quote(tmp) + " && mv -f " + quote(tmp) + " " + quote(dst) + " && chmod 644 " + quote(dst) + " && wc -c < " + quote(dst)
	done := make(chan struct{})
	var out string
	go func() {
		defer close(done)
		out, err = c.run(cmd, f)
	}()
	select {
	case <-ctx.Done():
		c.c.Close()
		return false, ctx.Err()
	case <-done:
	}
	if err != nil {
		return false, fmt.Errorf("upload failed: %w", err)
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if n != st.Size() {
		return false, fmt.Errorf("the server kept %d bytes of %d", n, st.Size())
	}
	return true, nil
}

// sameChecksum compares the local file with the remote one by SHA-256,
// falling back to "different" when the server has no sha256sum.
func (c *Client) sameChecksum(f *os.File, remote string) (bool, error) {
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	local := fmt.Sprintf("%x", h.Sum(nil))
	out, err := c.run("sha256sum "+quote(remote)+" 2>/dev/null | cut -d' ' -f1", nil)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == local, nil
}

// Fingerprint of a key for display, kept short.
func ShortFingerprint(fp string) string {
	fp = strings.TrimPrefix(fp, "SHA256:")
	if len(fp) > 12 {
		return fp[:12] + "…"
	}
	return fp
}
