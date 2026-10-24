// Package store keeps the inventory, credentials, templates, last snapshots
// and config copies as plain files in the user's config directory.
package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jaba0x/phonedesk/internal/grandstream"
)

type Phone struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Host         string    `json:"host"`
	Port         int       `json:"port"`   // 0 = default for the scheme
	Scheme       string    `json:"scheme"` // "auto", "https" or "http"
	CredentialID string    `json:"credentialId"`
	Site         string    `json:"site"`
	Notes        string    `json:"notes"`
	MAC          string    `json:"mac"`      // learned on the first read; used for cfg<mac>.xml
	Family       string    `json:"family"`   // firmware family override, "" = detect from the model
	Disabled     bool      `json:"disabled"` // paused: not polled, not read, no alerts
	CreatedAt    time.Time `json:"createdAt"`
}

type Credential struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"` // encrypted on disk, never sent to the UI
}

type Settings struct {
	PingInterval    int `json:"pingInterval"`    // seconds
	CollectInterval int `json:"collectInterval"` // minutes, 0 = manual only
	Parallel        int `json:"parallel"`
	KeepBackups     int `json:"keepBackups"`
	StaleHours      int `json:"staleHours"` // warn when data is older than this
	// Firmware: where phones fetch images from during an update.
	// "builtin" serves the library from this computer over HTTP,
	// "grandstream" points phones at firmware.grandstream.com,
	// "custom" uses FirmwareServer as the server path.
	// "relay" uploads the image over SSH to a server the phones can reach,
	// then points them at it.
	FirmwareSource string `json:"firmwareSource"`
	FirmwareServer string `json:"firmwareServer"`
	FirmwareVia    string `json:"firmwareVia"`  // "http", "https" or "tftp", for a custom server
	FirmwarePort   int    `json:"firmwarePort"` // built-in HTTP server port
	// The relay server. RelayPassword is encrypted on disk and never sent to
	// the UI; RelayHasPassword tells the UI one is set.
	RelayHost        string `json:"relayHost"`
	RelayPort        int    `json:"relayPort"`
	RelayUser        string `json:"relayUser"`
	RelayPassword    string `json:"relayPassword,omitempty"`
	RelayHasPassword bool   `json:"relayHasPassword"`
	RelayKeyFile     string `json:"relayKeyFile"`
	RelayHostKey     string `json:"relayHostKey"`    // pinned on first connect
	RelayDir         string `json:"relayDir"`        // where images land on the server, e.g. /tftpboot
	RelayServerPath  string `json:"relayServerPath"` // what phones use to reach it: host or host/folder
	RelayVia         string `json:"relayVia"`        // "tftp", "http" or "https"
	// ChecksOff lists the dashboard checks switched off, by id (see core.Checks).
	// Absent from the file means the defaults; an empty list means every check is on.
	ChecksOff []string `json:"checksOff"`
}

func DefaultSettings() Settings {
	return Settings{PingInterval: 30, CollectInterval: 15, Parallel: 4, KeepBackups: 20, StaleHours: 6,
		FirmwareSource: "builtin", FirmwareVia: "http", FirmwarePort: 6970,
		RelayPort: 22, RelayDir: "/tftpboot", RelayVia: "tftp",
		ChecksOff: []string{"option66"}}
}

type data struct {
	Phones      []Phone                `json:"phones"`
	Credentials []Credential           `json:"credentials"`
	Settings    Settings               `json:"settings"`
	Templates   []grandstream.Template `json:"templates"`
}

type Store struct {
	dir string
	key []byte
	mu  sync.Mutex
	d   data
}

func DefaultDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "PhoneDesk"), nil
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "snapshots"), 0o700); err != nil {
		return nil, err
	}
	os.MkdirAll(filepath.Join(dir, "backups"), 0o700)
	s := &Store{dir: dir}
	keyPath := filepath.Join(dir, "secret.key")
	key, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := os.WriteFile(keyPath, key, 0o600); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("%s is corrupt", keyPath)
	}
	s.key = key
	s.d.Settings = DefaultSettings()
	b, err := os.ReadFile(filepath.Join(dir, "inventory.json"))
	if err == nil {
		if err := json.Unmarshal(b, &s.d); err != nil {
			return nil, fmt.Errorf("inventory.json: %w", err)
		}
	}
	s.d.Settings = fixSettings(s.d.Settings)
	return s, nil
}

func fixSettings(v Settings) Settings {
	def := DefaultSettings()
	if v.PingInterval < 5 {
		v.PingInterval = def.PingInterval
	}
	if v.Parallel < 1 {
		v.Parallel = def.Parallel
	}
	if v.Parallel > 16 {
		v.Parallel = 16
	}
	if v.KeepBackups < 1 {
		v.KeepBackups = def.KeepBackups
	}
	if v.StaleHours < 1 {
		v.StaleHours = def.StaleHours
	}
	switch v.FirmwareSource {
	case "builtin", "grandstream", "custom":
	default:
		v.FirmwareSource = def.FirmwareSource
	}
	switch v.FirmwareVia {
	case "http", "https", "tftp":
	default:
		v.FirmwareVia = def.FirmwareVia
	}
	if v.FirmwarePort < 1 || v.FirmwarePort > 65535 {
		v.FirmwarePort = def.FirmwarePort
	}
	return v
}

func (s *Store) Dir() string { return s.dir }

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.d, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, "inventory.json.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, "inventory.json"))
}

// ---- secrets ----

func (s *Store) encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	blk, _ := aes.NewCipher(s.key)
	gcm, _ := cipher.NewGCM(blk)
	nonce := make([]byte, gcm.NonceSize())
	rand.Read(nonce)
	return "enc:" + base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plain), nil)), nil
}

func (s *Store) decrypt(v string) (string, error) {
	if !strings.HasPrefix(v, "enc:") {
		return v, nil
	}
	raw, err := base64.StdEncoding.DecodeString(v[4:])
	if err != nil {
		return "", err
	}
	blk, _ := aes.NewCipher(s.key)
	gcm, _ := cipher.NewGCM(blk)
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	p, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	return string(p), err
}

// ---- phones ----

func NewID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func (s *Store) Phones() []Phone {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]Phone(nil), s.d.Phones...)
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name+out[i].Host) < strings.ToLower(out[j].Name+out[j].Host)
	})
	return out
}

func (s *Store) Phone(id string) (Phone, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.d.Phones {
		if p.ID == id {
			return p, true
		}
	}
	return Phone{}, false
}

func (s *Store) SavePhone(p Phone) (Phone, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.Host = strings.TrimSpace(p.Host)
	if p.Host == "" {
		return p, errors.New("address is required")
	}
	switch p.Scheme {
	case "https", "http":
	default:
		p.Scheme = "auto"
	}
	if p.MAC != "" {
		p.MAC = grandstream.NormalizeMAC(p.MAC)
	}
	p.Family = strings.ToLower(strings.TrimSpace(p.Family))
	for _, o := range s.d.Phones {
		if o.ID != p.ID && strings.EqualFold(o.Host, p.Host) && o.Port == p.Port {
			return p, fmt.Errorf("%s is already in the inventory", p.Host)
		}
	}
	if p.ID == "" {
		p.ID = NewID()
		p.CreatedAt = time.Now()
		s.d.Phones = append(s.d.Phones, p)
	} else {
		found := false
		for i, o := range s.d.Phones {
			if o.ID == p.ID {
				if p.MAC == "" {
					p.MAC = o.MAC
				}
				p.CreatedAt = o.CreatedAt
				s.d.Phones[i] = p
				found = true
			}
		}
		if !found {
			return p, errors.New("phone not found")
		}
	}
	return p, s.saveLocked()
}

// SetMAC records the MAC learned from the phone.
func (s *Store) SetMAC(id, mac string) {
	mac = grandstream.NormalizeMAC(mac)
	if mac == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.d.Phones {
		if s.d.Phones[i].ID == id && s.d.Phones[i].MAC != mac {
			s.d.Phones[i].MAC = mac
			s.saveLocked()
		}
	}
}

// SetDisabled pauses or resumes the given phones.
func (s *Store) SetDisabled(ids []string, disabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	for i := range s.d.Phones {
		if want[s.d.Phones[i].ID] {
			s.d.Phones[i].Disabled = disabled
		}
	}
	return s.saveLocked()
}

func (s *Store) DeletePhone(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.d.Phones[:0]
	for _, p := range s.d.Phones {
		if p.ID != id {
			out = append(out, p)
		}
	}
	s.d.Phones = out
	os.Remove(filepath.Join(s.dir, "snapshots", id+".json"))
	os.RemoveAll(filepath.Join(s.dir, "backups", id))
	return s.saveLocked()
}

// ---- credentials ----

func (s *Store) Credentials() []Credential {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Credential, 0, len(s.d.Credentials))
	for _, c := range s.d.Credentials {
		c.Password = ""
		out = append(out, c)
	}
	return out
}

func (s *Store) Credential(id string) (Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.d.Credentials {
		if c.ID == id {
			p, err := s.decrypt(c.Password)
			if err != nil {
				return c, fmt.Errorf("can't decrypt password for %q: %w", c.Name, err)
			}
			c.Password = p
			return c, nil
		}
	}
	return Credential{}, errors.New("no credentials assigned")
}

// SaveCredential stores a credential set. An empty password on update keeps the old one.
func (s *Store) SaveCredential(c Credential) (Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.Username = strings.TrimSpace(c.Username)
	if c.Username == "" {
		c.Username = "admin"
	}
	if c.Name == "" {
		c.Name = c.Username
	}
	enc, err := s.encrypt(c.Password)
	if err != nil {
		return c, err
	}
	if c.ID == "" {
		if c.Password == "" {
			return c, errors.New("password is required")
		}
		c.ID = NewID()
		c.Password = enc
		s.d.Credentials = append(s.d.Credentials, c)
	} else {
		found := false
		for i, o := range s.d.Credentials {
			if o.ID == c.ID {
				if c.Password == "" {
					c.Password = o.Password
				} else {
					c.Password = enc
				}
				s.d.Credentials[i] = c
				found = true
			}
		}
		if !found {
			return c, errors.New("credential not found")
		}
	}
	out := c
	out.Password = ""
	return out, s.saveLocked()
}

func (s *Store) DeleteCredential(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.d.Phones {
		if p.CredentialID == id {
			return fmt.Errorf("still used by %s", firstNonEmpty(p.Name, p.Host))
		}
	}
	out := s.d.Credentials[:0]
	for _, c := range s.d.Credentials {
		if c.ID != id {
			out = append(out, c)
		}
	}
	s.d.Credentials = out
	return s.saveLocked()
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// ---- settings ----

// Settings returns the settings without the relay password; RelayHasPassword
// says whether one is stored.
func (s *Store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.d.Settings
	v.RelayHasPassword = v.RelayPassword != ""
	v.RelayPassword = ""
	return v
}

// RelayPassword returns the stored relay password in clear.
func (s *Store) RelayPassword() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.decrypt(s.d.Settings.RelayPassword)
}

// SetRelayHostKey pins (or clears) the relay server's host key.
func (s *Store) SetRelayHostKey(fp string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.d.Settings.RelayHostKey == fp {
		return nil
	}
	s.d.Settings.RelayHostKey = fp
	return s.saveLocked()
}

// SaveSettings stores v. An empty relay password keeps the one already stored.
func (s *Store) SaveSettings(v Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.FirmwareSource == "custom" && strings.TrimSpace(v.FirmwareServer) == "" {
		return errors.New("enter the firmware server path, or pick another source")
	}
	v.FirmwareServer = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(v.FirmwareServer, "https://"), "http://"))
	v.RelayHost, v.RelayUser, v.RelayDir, v.RelayKeyFile = strings.TrimSpace(v.RelayHost), strings.TrimSpace(v.RelayUser), strings.TrimSpace(v.RelayDir), strings.TrimSpace(v.RelayKeyFile)
	v.RelayServerPath = strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(v.RelayServerPath, "https://"), "http://")), "/")
	if v.FirmwareSource == "relay" {
		if v.RelayHost == "" || v.RelayUser == "" || v.RelayDir == "" || v.RelayServerPath == "" {
			return errors.New("the relay needs an SSH host, user, remote folder and the path phones use")
		}
	}
	if v.RelayPort <= 0 || v.RelayPort > 65535 {
		v.RelayPort = 22
	}
	switch v.RelayVia {
	case "tftp", "http", "https":
	default:
		v.RelayVia = "tftp"
	}
	if v.RelayPassword != "" {
		enc, err := s.encrypt(v.RelayPassword)
		if err != nil {
			return err
		}
		v.RelayPassword = enc
	} else {
		v.RelayPassword = s.d.Settings.RelayPassword
	}
	if v.RelayHost != s.d.Settings.RelayHost || v.RelayPort != s.d.Settings.RelayPort {
		v.RelayHostKey = "" // another server, another key
	} else {
		v.RelayHostKey = s.d.Settings.RelayHostKey
	}
	v.RelayHasPassword = false
	s.d.Settings = fixSettings(v)
	return s.saveLocked()
}

// ---- templates ----

func (s *Store) Templates() []grandstream.Template {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]grandstream.Template(nil), s.d.Templates...)
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func (s *Store) Template(id string) (grandstream.Template, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.d.Templates {
		if t.ID == id {
			return t, true
		}
	}
	return grandstream.Template{}, false
}

func (s *Store) SaveTemplate(t grandstream.Template) (grandstream.Template, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return t, errors.New("give the template a name")
	}
	if t.Values == nil {
		t.Values = map[string]string{}
	}
	for k := range t.Values {
		if !grandstream.IsPKey(k) {
			return t, fmt.Errorf("%q is not a P-value", k)
		}
	}
	t.UpdatedAt = time.Now()
	if t.ID == "" {
		t.ID = NewID()
		t.CreatedAt = t.UpdatedAt
		s.d.Templates = append(s.d.Templates, t)
	} else {
		found := false
		for i, o := range s.d.Templates {
			if o.ID == t.ID {
				t.CreatedAt = o.CreatedAt
				s.d.Templates[i] = t
				found = true
			}
		}
		if !found {
			return t, errors.New("template not found")
		}
	}
	return t, s.saveLocked()
}

func (s *Store) DeleteTemplate(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.d.Templates[:0]
	for _, t := range s.d.Templates {
		if t.ID != id {
			out = append(out, t)
		}
	}
	s.d.Templates = out
	return s.saveLocked()
}

// ---- snapshots & backups ----

func (s *Store) SaveSnapshot(id string, snap grandstream.Snapshot) error {
	b, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, "snapshots", id+".json"), b, 0o600)
}

func (s *Store) LoadSnapshot(id string) (grandstream.Snapshot, bool) {
	var snap grandstream.Snapshot
	b, err := os.ReadFile(filepath.Join(s.dir, "snapshots", id+".json"))
	if err != nil || json.Unmarshal(b, &snap) != nil {
		return snap, false
	}
	return snap, true
}

type Backup struct {
	Name string    `json:"name"`
	At   time.Time `json:"at"`
	Size int64     `json:"size"`
}

// SaveBackup writes the phone's settings (as "P47 = value" lines) if they
// differ from the newest copy. Status keys change on every read and are left
// out so a copy is only made when a setting really changed.
func (s *Store) SaveBackup(id string, values map[string]string, keep int) (bool, error) {
	settings := map[string]string{}
	for k, v := range values {
		if grandstream.IsPKey(k) {
			settings[k] = v
		}
	}
	if len(settings) == 0 {
		return false, nil
	}
	text := grandstream.FormatLines(settings)
	dir := filepath.Join(s.dir, "backups", id)
	os.MkdirAll(dir, 0o700)
	list := s.Backups(id)
	if len(list) > 0 {
		last, _ := os.ReadFile(filepath.Join(dir, list[0].Name))
		if strings.TrimSpace(string(last)) == strings.TrimSpace(text) {
			return false, nil
		}
	}
	stamp := time.Now().Format("20060102-150405")
	name := stamp + ".cfg"
	for i := 1; ; i++ { // two changes within a second
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			break
		}
		name = fmt.Sprintf("%s-%d.cfg", stamp, i)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
		return false, err
	}
	list = s.Backups(id)
	for i := keep; i < len(list); i++ {
		os.Remove(filepath.Join(dir, list[i].Name))
	}
	return true, nil
}

func (s *Store) Backups(id string) []Backup {
	dir := filepath.Join(s.dir, "backups", id)
	ents, _ := os.ReadDir(dir)
	var out []Backup
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".cfg") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".cfg")
		if len(base) > 15 {
			base = base[:15]
		}
		at, err := time.ParseInLocation("20060102-150405", base, time.Local)
		if err != nil {
			at = info.ModTime()
		}
		out = append(out, Backup{Name: e.Name(), At: at, Size: info.Size()})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.After(out[j].At)
		}
		return backupSeq(out[i].Name) > backupSeq(out[j].Name)
	})
	return out
}

// backupSeq is the "-n" suffix of a copy made within the same second.
func backupSeq(name string) int {
	base := strings.TrimSuffix(name, ".cfg")
	if i := strings.LastIndex(base, "-"); i > 8 {
		n := 0
		fmt.Sscanf(base[i+1:], "%d", &n)
		return n
	}
	return 0
}

func (s *Store) ReadBackup(id, name string) (string, error) {
	if strings.ContainsAny(name, `/\`) || !strings.HasSuffix(name, ".cfg") {
		return "", errors.New("bad backup name")
	}
	b, err := os.ReadFile(filepath.Join(s.dir, "backups", id, name))
	return string(b), err
}
