package relay

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// testServer is a tiny SSH server that runs each exec request through sh,
// which is what a Linux relay does with our commands.
func testServer(t *testing.T) (addr, fingerprint string) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == "u" && string(pass) == "p" {
				return nil, nil
			}
			return nil, errors.New("no")
		},
	}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serve(conn, cfg)
		}
	}()
	return ln.Addr().String(), ssh.FingerprintSHA256(signer.PublicKey())
}

func serve(conn net.Conn, cfg *ssh.ServerConfig) {
	sc, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sc.Close()
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			nc.Reject(ssh.UnknownChannelType, "")
			continue
		}
		ch, creqs, err := nc.Accept()
		if err != nil {
			return
		}
		go func() {
			defer ch.Close()
			for r := range creqs {
				if r.Type != "exec" {
					r.Reply(false, nil)
					continue
				}
				n := binary.BigEndian.Uint32(r.Payload)
				cmd := exec.Command("sh", "-c", string(r.Payload[4:4+n]))
				cmd.Stdin, cmd.Stdout, cmd.Stderr = ch, ch, ch.Stderr()
				r.Reply(true, nil)
				status := uint32(0)
				if err := cmd.Run(); err != nil {
					status = 1
				}
				var b [4]byte
				binary.BigEndian.PutUint32(b[:], status)
				ch.SendRequest("exit-status", false, b[:])
				return
			}
		}()
	}
}

func TestUploadAndCheck(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs sh")
	}
	addr, fp := testServer(t)
	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	for _, ch := range portStr {
		port = port*10 + int(ch-'0')
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if _, err := Dial(ctx, Options{Host: host, Port: port, User: "u", Password: "wrong"}); err == nil {
		t.Fatal("wrong password accepted")
	}
	if _, err := Dial(ctx, Options{Host: host, Port: port, User: "u", Password: "p", KnownKey: "SHA256:notthisone"}); !errors.Is(err, ErrHostKeyMismatch) {
		t.Fatalf("expected host key mismatch, got %v", err)
	}
	c, err := Dial(ctx, Options{Host: host, Port: port, User: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.Fingerprint != fp {
		t.Fatalf("fingerprint %q, want %q", c.Fingerprint, fp)
	}

	remote := filepath.Join(t.TempDir(), "tftpboot")
	if _, err := c.Check(remote); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(t.TempDir(), "grp2610fw-1.0.13.31.bin")
	data := make([]byte, 300000)
	rand.Read(data)
	os.WriteFile(local, data, 0o600)

	dir := remote + "/phonedesk/1.0.13.31"
	copied, err := c.Upload(ctx, local, dir, "grp2610fw.bin")
	if err != nil || !copied {
		t.Fatalf("first upload: copied=%v err=%v", copied, err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "grp2610fw.bin"))
	if string(got) != string(data) {
		t.Fatal("remote file differs")
	}
	copied, err = c.Upload(ctx, local, dir, "grp2610fw.bin")
	if err != nil || copied {
		t.Fatalf("second upload should be skipped: copied=%v err=%v", copied, err)
	}
	data[0] ^= 0xff
	os.WriteFile(local, data, 0o600)
	copied, err = c.Upload(ctx, local, dir, "grp2610fw.bin")
	if err != nil || !copied {
		t.Fatalf("changed file should be copied again: copied=%v err=%v", copied, err)
	}
	if n, err := c.RemoteSize(filepath.Join(dir, "grp2610fw.bin")); err != nil || n != int64(len(data)) {
		t.Fatalf("remote size %d %v", n, err)
	}
	if n, _ := c.RemoteSize(filepath.Join(dir, "missing.bin")); n != -1 {
		t.Fatalf("missing file size %d", n)
	}
}
