package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jaba0x/phonedesk/internal/store"
)

type fakePhone struct {
	values   map[string]string
	rebooted int
	lines    bool
}

func (p *fakePhone) handler() http.Handler {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, body any) {
		json.NewEncoder(w).Encode(map[string]any{"response": "success", "body": body})
	}
	fail := func(w http.ResponseWriter, body string) {
		json.NewEncoder(w).Encode(map[string]any{"response": "error", "body": body})
	}
	mux.HandleFunc("/cgi-bin/dologin", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("password") != "admin" {
			fail(w, "unauthorized")
			return
		}
		ok(w, map[string]any{"sid": "s1"})
	})
	mux.HandleFunc("/cgi-bin/api.values.get", func(w http.ResponseWriter, r *http.Request) {
		out := map[string]string{}
		for _, k := range strings.Split(r.FormValue("request"), ":") {
			if v, ok := p.values[k]; ok {
				out[k] = v
			}
		}
		ok(w, out)
	})
	mux.HandleFunc("/cgi-bin/api.values.post", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		for k, v := range r.PostForm {
			if strings.HasPrefix(k, "P") {
				p.values[k] = v[0]
			}
		}
		ok(w, map[string]string{"status": "right"})
	})
	mux.HandleFunc("/cgi-bin/api-sys_operation", func(w http.ResponseWriter, r *http.Request) {
		p.rebooted++
		ok(w, "savereboot")
	})
	mux.HandleFunc("/cgi-bin/api-get_line_status", func(w http.ResponseWriter, r *http.Request) {
		if !p.lines {
			fail(w, "unauthorized")
			return
		}
		ok(w, []map[string]any{{"line": 1, "state": "idle", "acct": 0}, {"line": 2, "state": "failed", "acct": 1}})
	})
	return mux
}

func setup(t *testing.T, p *fakePhone) (*Manager, *store.Store, store.Phone) {
	probeTimeout = 300 * time.Millisecond
	httpPortNum, httpsPortNum, sshPortNum, telnetPortNum = "1", "1", "1", "1" // nothing listens there
	srv := httptest.NewServer(p.handler())
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c, _ := st.SaveCredential(store.Credential{Username: "admin", Password: "admin"})
	ph, _ := st.SavePhone(store.Phone{Name: "Lobby", Host: u.Hostname(), Port: port, Scheme: "http", CredentialID: c.ID})
	m := New(st, nil)
	m.ctx, m.stop = context.WithCancel(context.Background())
	return m, st, ph
}

func TestReadAndIssues(t *testing.T) {
	p := &fakePhone{lines: true, values: map[string]string{"Pphone_model": "GRP2613", "prog": "1.0.13.31", "mac": "c074ad112233", "P271": "1", "P35": "201", "P47": "pbx.lan", "P401": "1", "P404": "202", "P402": "pbx.lan", "P207": "", "P145": "1"}}
	m, st, ph := setup(t, p)
	m.collect(ph.ID)
	d, err := m.Detail(ph.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status.CollectError != "" {
		t.Fatalf("collect error: %s", d.Status.CollectError)
	}
	if d.Summary == nil || d.Summary.Model != "GRP2613" || d.Summary.Extension != "201" || d.Summary.ActiveAccounts != 2 || d.Summary.Registered != 1 {
		t.Fatalf("summary %+v", d.Summary)
	}
	if got, _ := st.Phone(ph.ID); got.MAC != "c0:74:ad:11:22:33" {
		t.Fatalf("mac not learned: %q", got.MAC)
	}
	titles := map[string]bool{}
	for _, i := range d.Issues {
		titles[i.Title] = true
	}
	for _, want := range []string{"Account 2 (202) is not registered", "Default admin password", "Web access over plain HTTP", "No NTP server", "No syslog server"} {
		if !titles[want] {
			t.Errorf("missing issue %q in %v", want, titles)
		}
	}
	if d.Worst != "critical" {
		t.Fatalf("worst %q", d.Worst)
	}
	if len(st.Backups(ph.ID)) != 1 {
		t.Fatal("no config copy saved")
	}
	dash := m.Dashboard()
	if dash.Total != 1 || dash.Critical < 2 || dash.Registered != 1 || dash.Accounts != 2 {
		t.Fatalf("dashboard %+v", dash)
	}

	// apply a template: NTP set, account 2 already right
	pv := m.Preview([]string{ph.ID}, map[string]string{"P30": "pool.ntp.org", "P404": "202"})
	if len(pv) != 1 || pv[0].Changed != 1 {
		t.Fatalf("preview %+v", pv)
	}
	res := m.Apply([]string{ph.ID}, map[string]string{"P30": "pool.ntp.org", "P404": "202"}, false)
	if len(res) != 1 || !res[0].OK || res[0].Written != 1 {
		t.Fatalf("apply %+v", res)
	}
	if p.values["P30"] != "pool.ntp.org" {
		t.Fatal("value not written to the phone")
	}
	d, _ = m.Detail(ph.ID)
	if d.Snapshot.Time.NTPServer != "pool.ntp.org" {
		t.Fatal("snapshot not re-read after apply")
	}
	for _, i := range d.Issues {
		if i.Title == "No NTP server" {
			t.Fatal("NTP issue should be gone")
		}
	}
	if len(st.Backups(ph.ID)) != 2 {
		t.Fatal("second copy expected after a change")
	}
	res = m.Apply([]string{ph.ID}, map[string]string{"P30": "pool.ntp.org"}, true)
	if !res[0].OK || res[0].Written != 0 || p.rebooted != 0 {
		t.Fatalf("no-op apply %+v rebooted=%d", res, p.rebooted)
	}

	files, missing := m.ProvisioningFiles([]string{ph.ID}, map[string]string{"P30": "x"})
	if len(missing) != 0 || len(files) != 1 {
		t.Fatalf("files %v missing %v", files, missing)
	}
	if _, ok := files["cfgc074ad112233.xml"]; !ok {
		t.Fatalf("file name %v", files)
	}
	if err := m.Reboot(ph.ID); err != nil || p.rebooted != 1 {
		t.Fatalf("reboot %v %d", err, p.rebooted)
	}
	ext := m.Extensions()
	if len(ext) != 2 || ext[0].UserID != "201" {
		t.Fatalf("extensions %+v", ext)
	}
}

func TestAuthFailure(t *testing.T) {
	p := &fakePhone{values: map[string]string{}}
	m, st, ph := setup(t, p)
	c, _ := st.SaveCredential(store.Credential{Username: "admin", Password: "wrong"})
	ph.CredentialID = c.ID
	st.SavePhone(ph)
	m.collect(ph.ID)
	d, _ := m.Detail(ph.ID)
	if !d.Status.AuthFailed {
		t.Fatalf("expected auth failure, got %+v", d.Status)
	}
	if d.Issues[0].Title != "Web login failed" {
		t.Fatalf("issues %+v", d.Issues)
	}
	if _, err := m.TestConnection(ph); err == nil {
		t.Fatal("test connection should fail")
	}
}

func TestScanAndTargets(t *testing.T) {
	hosts, err := ExpandTargets("10.0.1.0/30, 10.0.2.10-12\n10.0.3.5")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(hosts, " ") != "10.0.1.1 10.0.1.2 10.0.2.10 10.0.2.11 10.0.2.12 10.0.3.5" {
		t.Fatalf("%v", hosts)
	}
	if _, err := ExpandTargets("10.0.0.0/8"); err == nil {
		t.Fatal("huge range accepted")
	}
	p := &fakePhone{values: map[string]string{}}
	m, _, ph := setup(t, p)
	var last ScanProgress
	found := m.Scan(context.Background(), []string{ph.Host + ":" + strconv.Itoa(ph.Port)}, func(sp ScanProgress) { last = sp })
	// the scanner needs host:port here because the fake listens on a random port
	if len(found) != 1 || found[0].Scheme != "http" || last.Done != 1 {
		t.Fatalf("found %+v progress %+v", found, last)
	}
}

func TestFleetIssues(t *testing.T) {
	p := &fakePhone{values: map[string]string{"P271": "1", "P35": "300", "P47": "pbx"}}
	m, st, ph := setup(t, p)
	m.collect(ph.ID)
	ph2, _ := st.SavePhone(store.Phone{Name: "Desk 2", Host: ph.Host, Port: ph.Port + 100000, Scheme: "http", CredentialID: ph.CredentialID})
	snap := *m.Snapshot(ph.ID)
	m.mu.Lock()
	m.snaps[ph2.ID] = &snap
	m.mu.Unlock()
	d := m.Dashboard()
	found := false
	for _, i := range d.Issues {
		if strings.HasPrefix(i.Title, "Extension 300 is on 2 phones") {
			found = true
		}
	}
	if !found {
		t.Fatalf("fleet issue missing: %+v", d.Issues)
	}
}
