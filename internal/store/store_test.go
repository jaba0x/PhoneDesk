package store

import (
	"path/filepath"
	"testing"

	"github.com/jaba0x/phonedesk/internal/grandstream"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.SaveCredential(Credential{Name: "Phones", Username: "admin", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Password != "" {
		t.Fatal("password returned to caller")
	}
	got, err := s.Credential(c.ID)
	if err != nil || got.Password != "secret" {
		t.Fatalf("credential %+v %v", got, err)
	}
	p, err := s.SavePhone(Phone{Host: "10.0.0.21", CredentialID: c.ID, Scheme: "weird"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Scheme != "auto" || p.ID == "" {
		t.Fatalf("phone %+v", p)
	}
	if _, err := s.SavePhone(Phone{Host: "10.0.0.21"}); err == nil {
		t.Fatal("duplicate host accepted")
	}
	s.SetMAC(p.ID, "C0-74-AD-00-11-22")
	tp, _ := s.Phone(p.ID)
	if tp.MAC != "c0:74:ad:00:11:22" {
		t.Fatalf("mac %q", tp.MAC)
	}
	if err := s.DeleteCredential(c.ID); err == nil {
		t.Fatal("credential in use was deleted")
	}

	tpl, err := s.SaveTemplate(grandstream.Template{Name: "Office", Values: map[string]string{"P30": "pool.ntp.org"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveTemplate(grandstream.Template{Name: "Bad", Values: map[string]string{"x": "1"}}); err == nil {
		t.Fatal("bad key accepted")
	}

	ok, err := s.SaveBackup(p.ID, map[string]string{"P30": "a", "prog": "1.0"}, 3)
	if err != nil || !ok {
		t.Fatalf("backup %v %v", ok, err)
	}
	if ok, _ := s.SaveBackup(p.ID, map[string]string{"P30": "a", "prog": "2.0"}, 3); ok {
		t.Fatal("status-only change made a copy")
	}
	if ok, _ := s.SaveBackup(p.ID, map[string]string{"P30": "b"}, 3); !ok {
		t.Fatal("changed setting made no copy")
	}
	if len(s.Backups(p.ID)) != 2 {
		t.Fatal("expected two copies")
	}
	txt, err := s.ReadBackup(p.ID, s.Backups(p.ID)[0].Name)
	if err != nil || txt != "# NTP server\nP30 = b\n" {
		t.Fatalf("backup text %q %v", txt, err)
	}
	if _, err := s.ReadBackup(p.ID, "../x.cfg"); err == nil {
		t.Fatal("path escape accepted")
	}

	// reopen: everything persisted
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s2.Phones()) != 1 || len(s2.Templates()) != 1 || s2.Templates()[0].ID != tpl.ID {
		t.Fatal("data not persisted")
	}
	if got, _ := s2.Credential(c.ID); got.Password != "secret" {
		t.Fatal("password not decrypted after reopen")
	}

	// export and import into a fresh folder
	bundle := filepath.Join(t.TempDir(), "x.pdbak")
	if err := s2.Export(bundle, "passphrase1", true); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectBundle(bundle, "wrong-pass"); err != ErrBadPassphrase {
		t.Fatalf("wrong passphrase: %v", err)
	}
	info, err := InspectBundle(bundle, "passphrase1")
	if err != nil || info.Phones != 1 || !info.History {
		t.Fatalf("inspect %+v %v", info, err)
	}
	s3, _ := Open(t.TempDir())
	res, err := s3.Import(bundle, "passphrase1", "merge")
	if err != nil || res.Added != 1 || res.Credentials != 1 {
		t.Fatalf("import %+v %v", res, err)
	}
	if got, _ := s3.Credential(c.ID); got.Password != "secret" {
		t.Fatal("imported password unreadable")
	}
	if len(s3.Templates()) != 1 || len(s3.Backups(p.ID)) != 2 {
		t.Fatal("templates or copies not imported")
	}
	res, _ = s3.Import(bundle, "passphrase1", "merge")
	if res.Added != 0 || len(res.Skipped) != 1 {
		t.Fatalf("second merge %+v", res)
	}
}

func TestSettings(t *testing.T) {
	s, _ := Open(t.TempDir())
	if err := s.SaveSettings(Settings{FirmwareSource: "custom"}); err == nil {
		t.Fatal("custom source without a server accepted")
	}
	if err := s.SaveSettings(Settings{FirmwareSource: "custom", FirmwareServer: "http://fw.example.com/gs", PingInterval: 1}); err != nil {
		t.Fatal(err)
	}
	set := s.Settings()
	if set.FirmwareServer != "fw.example.com/gs" || set.PingInterval != 30 || set.FirmwarePort != 6970 {
		t.Fatalf("settings %+v", set)
	}
}

func TestRelaySettingsRoundTrip(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	set := st.Settings()
	set.FirmwareSource, set.RelayHost, set.RelayUser, set.RelayPassword, set.RelayDir, set.RelayServerPath, set.RelayVia = "relay", "10.0.18.100", "root", "s3cret", "/tftpboot", "10.0.18.100", "tftp"
	if err := st.SaveSettings(set); err != nil {
		t.Fatal(err)
	}
	got := st.Settings()
	if got.FirmwareSource != "relay" || got.RelayHost != "10.0.18.100" || got.RelayUser != "root" || got.RelayServerPath != "10.0.18.100" {
		t.Fatalf("relay settings not kept: %+v", got)
	}
	if got.RelayPassword != "" || !got.RelayHasPassword {
		t.Fatalf("password should be hidden from the UI but marked as set: %+v", got)
	}
	if pw, err := st.RelayPassword(); err != nil || pw != "s3cret" {
		t.Fatalf("stored password %q %v", pw, err)
	}
	// saving again with an empty password keeps the old one; a new host drops the pinned key
	st.SetRelayHostKey("SHA256:abc")
	set = st.Settings()
	set.RelayPassword = ""
	if err := st.SaveSettings(set); err != nil {
		t.Fatal(err)
	}
	if pw, _ := st.RelayPassword(); pw != "s3cret" {
		t.Fatal("empty password should keep the stored one")
	}
	if st.Settings().RelayHostKey != "SHA256:abc" {
		t.Fatal("host key should survive a save for the same server")
	}
	set = st.Settings()
	set.RelayHost = "10.0.18.101"
	st.SaveSettings(set)
	if st.Settings().RelayHostKey != "" {
		t.Fatal("host key should be dropped for another server")
	}
	// relay chosen without a host is refused
	set = st.Settings()
	set.RelayHost = ""
	if err := st.SaveSettings(set); err == nil {
		t.Fatal("relay without a host should be refused")
	}
}
