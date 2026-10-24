package store

import (
	"archive/zip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// A bundle is a zip with a manifest, the inventory (passwords re-encrypted
// with a key derived from the passphrase) and optionally snapshots and config
// copies. It carries no machine key, so it opens on any computer.

const bundleFormat = 1

type manifest struct {
	App       string    `json:"app"`
	Format    int       `json:"format"`
	CreatedAt time.Time `json:"createdAt"`
	Salt      string    `json:"salt"`
	Check     string    `json:"check"` // encrypts a known value so a wrong passphrase is caught early
	Phones    int       `json:"phones"`
	History   bool      `json:"history"`
}

type BundleInfo struct {
	CreatedAt time.Time `json:"createdAt"`
	Phones    int       `json:"phones"`
	History   bool      `json:"history"`
}

func deriveKey(pass string, salt []byte) []byte {
	return argon2.IDKey([]byte(pass), salt, 3, 64*1024, 2, 32)
}

func sealWith(key []byte, plain string) string {
	blk, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(blk)
	nonce := make([]byte, gcm.NonceSize())
	rand.Read(nonce)
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plain), nil))
}

func openWith(key []byte, v string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return "", err
	}
	blk, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(blk)
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("short ciphertext")
	}
	p, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	return string(p), err
}

var ErrBadPassphrase = errors.New("wrong passphrase")

// Export writes a bundle to path.
func (s *Store) Export(dst, pass string, history bool) error {
	if len(pass) < 8 {
		return errors.New("use a passphrase of at least 8 characters")
	}
	salt := make([]byte, 16)
	rand.Read(salt)
	key := deriveKey(pass, salt)

	s.mu.Lock()
	d := s.d
	d.Credentials = make([]Credential, len(s.d.Credentials))
	for i, c := range s.d.Credentials {
		p, err := s.decrypt(c.Password)
		if err != nil {
			s.mu.Unlock()
			return fmt.Errorf("can't read password for %q: %w", c.Name, err)
		}
		c.Password = "bundle:" + sealWith(key, p)
		d.Credentials[i] = c
	}
	inv, err := json.MarshalIndent(d, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return err
	}

	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	write := func(name string, b []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	}
	m := manifest{App: "PhoneDesk", Format: bundleFormat, CreatedAt: time.Now(), Salt: base64.StdEncoding.EncodeToString(salt), Check: sealWith(key, "phonedesk"), Phones: len(d.Phones), History: history}
	mb, _ := json.MarshalIndent(m, "", "  ")
	err = write("manifest.json", mb)
	if err == nil {
		err = write("inventory.json", inv)
	}
	if err == nil && history {
		for _, sub := range []string{"snapshots", "backups"} {
			root := filepath.Join(s.dir, sub)
			err = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				rel, _ := filepath.Rel(s.dir, p)
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				return write(filepath.ToSlash(rel), b)
			})
			if err != nil {
				break
			}
		}
	}
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	os.Remove(dst)
	return os.Rename(tmp, dst)
}

type bundle struct {
	m     manifest
	key   []byte
	d     data
	files map[string]*zip.File // snapshots/... and backups/...
	zr    *zip.ReadCloser
}

func openBundle(src, pass string) (*bundle, error) {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return nil, errors.New("this isn't a PhoneDesk export file")
	}
	b := &bundle{zr: zr, files: map[string]*zip.File{}}
	read := func(f *zip.File) ([]byte, error) {
		if f.UncompressedSize64 > 200<<20 {
			return nil, errors.New("file inside the export is too large")
		}
		r, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(r)
	}
	var invFile *zip.File
	for _, f := range zr.File {
		name := path.Clean(f.Name)
		switch {
		case name == "manifest.json":
			raw, err := read(f)
			if err != nil || json.Unmarshal(raw, &b.m) != nil {
				zr.Close()
				return nil, errors.New("the export's manifest is damaged")
			}
		case name == "inventory.json":
			invFile = f
		case (strings.HasPrefix(name, "snapshots/") || strings.HasPrefix(name, "backups/")) && !strings.Contains(name, "..") && !path.IsAbs(name):
			b.files[name] = f
		}
	}
	if b.m.App != "PhoneDesk" || invFile == nil {
		zr.Close()
		return nil, errors.New("this isn't a PhoneDesk export file")
	}
	if b.m.Format > bundleFormat {
		zr.Close()
		return nil, errors.New("this export was made by a newer PhoneDesk; update the app first")
	}
	salt, _ := base64.StdEncoding.DecodeString(b.m.Salt)
	b.key = deriveKey(pass, salt)
	if v, err := openWith(b.key, b.m.Check); err != nil || v != "phonedesk" {
		zr.Close()
		return nil, ErrBadPassphrase
	}
	raw, err := read(invFile)
	if err == nil {
		err = json.Unmarshal(raw, &b.d)
	}
	if err != nil {
		zr.Close()
		return nil, errors.New("the export's inventory is damaged")
	}
	for i, c := range b.d.Credentials {
		if strings.HasPrefix(c.Password, "bundle:") {
			p, err := openWith(b.key, strings.TrimPrefix(c.Password, "bundle:"))
			if err != nil {
				zr.Close()
				return nil, ErrBadPassphrase
			}
			b.d.Credentials[i].Password = p
		}
	}
	return b, nil
}

// InspectBundle checks the passphrase and returns what's inside.
func InspectBundle(src, pass string) (BundleInfo, error) {
	b, err := openBundle(src, pass)
	if err != nil {
		return BundleInfo{}, err
	}
	defer b.zr.Close()
	return BundleInfo{CreatedAt: b.m.CreatedAt, Phones: len(b.d.Phones), History: b.m.History}, nil
}

type ImportResult struct {
	Added       int      `json:"added"`
	Skipped     []string `json:"skipped"`
	Credentials int      `json:"credentials"`
}

// Import loads a bundle. "replace" swaps the whole inventory; "merge" adds
// phones and credentials that aren't here yet (matched by address and port).
func (s *Store) Import(src, pass, mode string) (ImportResult, error) {
	res := ImportResult{}
	b, err := openBundle(src, pass)
	if err != nil {
		return res, err
	}
	defer b.zr.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	extract := func(oldID, newID string) {
		for name, f := range b.files {
			var target string
			switch {
			case name == "snapshots/"+oldID+".json":
				target = filepath.Join(s.dir, "snapshots", newID+".json")
			case strings.HasPrefix(name, "backups/"+oldID+"/") && strings.HasSuffix(name, ".cfg"):
				target = filepath.Join(s.dir, "backups", newID, path.Base(name))
			default:
				continue
			}
			os.MkdirAll(filepath.Dir(target), 0o700)
			r, err := f.Open()
			if err != nil {
				continue
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
			if err == nil {
				io.Copy(out, io.LimitReader(r, 200<<20))
				out.Close()
			}
			r.Close()
		}
	}

	switch mode {
	case "replace":
		creds := make([]Credential, 0, len(b.d.Credentials))
		for _, c := range b.d.Credentials {
			enc, err := s.encrypt(c.Password)
			if err != nil {
				return res, err
			}
			c.Password = enc
			creds = append(creds, c)
		}
		os.RemoveAll(filepath.Join(s.dir, "snapshots"))
		os.RemoveAll(filepath.Join(s.dir, "backups"))
		os.MkdirAll(filepath.Join(s.dir, "snapshots"), 0o700)
		os.MkdirAll(filepath.Join(s.dir, "backups"), 0o700)
		s.d.Phones = b.d.Phones
		s.d.Credentials = creds
		s.d.Settings = b.d.Settings
		s.d.Settings.RelayPassword, s.d.Settings.RelayHostKey = "", "" // encrypted with the other computer's key
		s.d.Templates = b.d.Templates
		for _, sw := range s.d.Phones {
			extract(sw.ID, sw.ID)
		}
		res.Added = len(s.d.Phones)
		res.Credentials = len(creds)
	case "merge":
		credMap := map[string]string{}
		for _, c := range b.d.Credentials {
			found := ""
			for _, o := range s.d.Credentials {
				if o.ID == c.ID {
					found = o.ID
				}
			}
			if found != "" {
				credMap[c.ID] = found
				continue
			}
			enc, err := s.encrypt(c.Password)
			if err != nil {
				return res, err
			}
			c.Password = enc
			s.d.Credentials = append(s.d.Credentials, c)
			credMap[c.ID] = c.ID
			res.Credentials++
		}
		for _, t := range b.d.Templates {
			exists := false
			for _, o := range s.d.Templates {
				if o.ID == t.ID || strings.EqualFold(o.Name, t.Name) {
					exists = true
				}
			}
			if !exists {
				s.d.Templates = append(s.d.Templates, t)
			}
		}
		for _, sw := range b.d.Phones {
			dup := false
			for _, o := range s.d.Phones {
				if strings.EqualFold(o.Host, sw.Host) && o.Port == sw.Port {
					dup = true
				}
			}
			if dup {
				res.Skipped = append(res.Skipped, sw.Host+" is already here")
				continue
			}
			oldID := sw.ID
			for _, o := range s.d.Phones {
				if o.ID == sw.ID {
					sw.ID = NewID()
				}
			}
			sw.CredentialID = credMap[sw.CredentialID]
			s.d.Phones = append(s.d.Phones, sw)
			extract(oldID, sw.ID)
			res.Added++
		}
	default:
		return res, fmt.Errorf("unknown import mode %q", mode)
	}
	return res, s.saveLocked()
}
