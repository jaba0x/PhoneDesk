package grandstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// fakePhone behaves like the web API of a GXP/GRP phone.
type fakePhone struct {
	password string
	values   map[string]string
	sid      string
	rebooted int
}

func (p *fakePhone) handler() http.Handler {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, body any) {
		json.NewEncoder(w).Encode(map[string]any{"response": "success", "body": body})
	}
	fail := func(w http.ResponseWriter, body string) {
		json.NewEncoder(w).Encode(map[string]any{"response": "error", "body": body})
	}
	authed := func(r *http.Request) bool {
		if r.FormValue("sid") == p.sid && p.sid != "" {
			return true
		}
		if c, err := r.Cookie("session-identity"); err == nil && c.Value == p.sid && p.sid != "" {
			return true
		}
		return false
	}
	mux.HandleFunc("/cgi-bin/dologin", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.FormValue("username") != "admin" || r.FormValue("password") != p.password {
			fail(w, "unauthorized")
			return
		}
		p.sid = "abc123"
		ok(w, map[string]any{"sid": p.sid, "role": "admin"})
	})
	mux.HandleFunc("/cgi-bin/api.values.get", func(w http.ResponseWriter, r *http.Request) {
		if !authed(r) {
			fail(w, "unauthorized")
			return
		}
		out := map[string]string{}
		for _, k := range strings.Split(r.FormValue("request"), ":") {
			if v, ok := p.values[k]; ok {
				out[k] = v
			}
		}
		ok(w, out)
	})
	mux.HandleFunc("/cgi-bin/api.values.post", func(w http.ResponseWriter, r *http.Request) {
		if !authed(r) {
			fail(w, "unauthorized")
			return
		}
		r.ParseForm()
		for k, v := range r.PostForm {
			if strings.HasPrefix(k, "P") {
				if k == "P9999" {
					fail(w, "invalid value")
					return
				}
				p.values[k] = v[0]
			}
		}
		ok(w, map[string]string{"status": "right"})
	})
	mux.HandleFunc("/cgi-bin/api-sys_operation", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("passcode") != p.password {
			fail(w, "unauthorized")
			return
		}
		if r.FormValue("request") == "REBOOT" {
			p.rebooted++
			ok(w, "savereboot")
			return
		}
		fail(w, "unknown")
	})
	mux.HandleFunc("/cgi-bin/api-get_line_status", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("passcode") != p.password {
			fail(w, "unauthorized")
			return
		}
		ok(w, []map[string]any{
			{"line": 1, "state": "idle", "acct": 0, "active": 0, "remotename": "", "remotenumber": ""},
			{"line": 2, "state": "8", "acct": 1, "active": 0},
		})
	})
	return mux
}

func newClient(t *testing.T, srv *httptest.Server, pass string) *Client {
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	return &Client{Host: u.Hostname(), Port: port, Scheme: "http", Username: "admin", Password: pass}
}

func TestLoginReadWrite(t *testing.T) {
	p := &fakePhone{password: "admin", values: map[string]string{"Pphone_model": "GRP2613", "prog": "1.0.13.31", "mac": "C0:74:AD:11:22:33", "P271": "1", "P35": "201", "P47": "pbx.example.com", "P401": "1", "P404": "202", "P402": "pbx.example.com", "P30": "pool.ntp.org"}}
	srv := httptest.NewServer(p.handler())
	defer srv.Close()
	ctx := context.Background()

	bad := newClient(t, srv, "wrong")
	if err := bad.Login(ctx); err != ErrAuth {
		t.Fatalf("wrong password: got %v, want ErrAuth", err)
	}
	c := newClient(t, srv, "admin")
	if err := c.Login(ctx); err != nil {
		t.Fatal(err)
	}
	if c.UsedScheme() != "http" {
		t.Fatalf("scheme %q", c.UsedScheme())
	}
	vals, err := c.Values(ctx, CollectKeys())
	if err != nil {
		t.Fatal(err)
	}
	if vals["P47"] != "pbx.example.com" || vals["prog"] != "1.0.13.31" {
		t.Fatalf("values %v", vals)
	}
	if _, ok := vals["P999999"]; ok {
		t.Fatal("unknown key should be absent")
	}
	if err := c.SetValues(ctx, map[string]string{"P30": "time.google.com"}); err != nil {
		t.Fatal(err)
	}
	if p.values["P30"] != "time.google.com" {
		t.Fatal("value not written")
	}
	if err := c.SetValues(ctx, map[string]string{"P9999": "x"}); err == nil {
		t.Fatal("refused value should error")
	}
	if err := c.SetValues(ctx, map[string]string{"nope": "x"}); err == nil {
		t.Fatal("non P key should be refused before sending")
	}
	lines, err := c.LineStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[1].State != "failed" || lines[0].State != "idle" {
		t.Fatalf("lines %+v", lines)
	}
	if err := c.Reboot(ctx); err != nil || p.rebooted != 1 {
		t.Fatalf("reboot: %v %d", err, p.rebooted)
	}
	raw := c.Raw()
	if _, ok := raw["dologin (http)"]; !ok {
		t.Fatalf("raw log missing login: %v", raw)
	}
	if strings.Contains(raw["dologin (http)"], "abc123") {
		t.Fatal("sid leaked into raw log")
	}

	snap := Build(vals, lines, true)
	if snap.Facts.Model != "GRP2613" || snap.Facts.Family != "grp2610" || snap.Facts.MAC != "c0:74:ad:11:22:33" {
		t.Fatalf("facts %+v", snap.Facts)
	}
	if len(snap.Accounts) != 2 {
		t.Fatalf("accounts %+v", snap.Accounts)
	}
	if snap.Accounts[0].Registered != 1 || snap.Accounts[1].Registered != 0 {
		t.Fatalf("registration %+v", snap.Accounts)
	}
	if snap.Time.NTPServer != "pool.ntp.org" {
		t.Fatalf("ntp %q", snap.Time.NTPServer)
	}

	scheme, err := Fingerprint(ctx, u(srv), 0)
	if err != nil || scheme != "http" {
		t.Fatalf("fingerprint %q %v", scheme, err)
	}
}

func u(srv *httptest.Server) string {
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestFingerprintNotGS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>Hello</html>"))
	}))
	defer srv.Close()
	if _, err := Fingerprint(context.Background(), u(srv), 0); err != ErrNotGS {
		t.Fatalf("got %v", err)
	}
}

func TestFamily(t *testing.T) {
	cases := map[string]string{"GRP2613": "grp2610", "GXP2170": "gxp2130", "GXP1628": "gxp1600", "Grandstream GRP2602W": "grp2600", "GRP2670": "grp2670", "WP822": "wp822", "XYZ-9": "xyz9", "": ""}
	for in, want := range cases {
		if got := Family(in); got != want {
			t.Errorf("Family(%q) = %q, want %q", in, got, want)
		}
	}
	if FirmwareFile("grp2610") != "grp2610fw.bin" {
		t.Fatal("firmware file name")
	}
}

func TestTemplates(t *testing.T) {
	vals, err := ParseLines("# ntp\nP30 = pool.ntp.org\nP64: TZR-4\np212=1\n\n")
	if err != nil {
		t.Fatal(err)
	}
	if vals["P30"] != "pool.ntp.org" || vals["P64"] != "TZR-4" || vals["P212"] != "1" {
		t.Fatalf("%v", vals)
	}
	if _, err := ParseLines("hello"); err == nil {
		t.Fatal("bad line should error")
	}
	txt := FormatLines(vals)
	if !strings.Contains(txt, "# NTP server\nP30 = pool.ntp.org") {
		t.Fatalf("format:\n%s", txt)
	}
	ch := Diff(map[string]string{"P30": "pool.ntp.org", "P64": "EST5EDT"}, map[string]string{"P30": "pool.ntp.org", "P64": "TZR-4", "P2": "secret"})
	if len(ch) != 3 || ch[0].Key != "P2" || !ch[0].Changed || ch[0].To != "••••••" {
		t.Fatalf("diff %+v", ch)
	}
	if ch[1].Changed || !ch[2].Changed {
		t.Fatalf("diff %+v", ch)
	}
	x := string(ProvisioningXML(map[string]string{"P47": "pbx<1>", "P30": "a"}, "C0:74:AD:11:22:33"))
	if !strings.Contains(x, "<P30>a</P30>") || !strings.Contains(x, "<P47>pbx&lt;1&gt;</P47>") || !strings.Contains(x, "<gs_provision") {
		t.Fatalf("xml:\n%s", x)
	}
	if strings.Index(x, "<P30>") > strings.Index(x, "<P47>") {
		t.Fatal("xml not in numeric order")
	}
	if ConfigFileName("C0:74:AD:11:22:33") != "cfgc074ad112233.xml" {
		t.Fatal("config file name")
	}
}

func TestSortKeys(t *testing.T) {
	keys := []string{"prog", "P47", "P2", "mac", "P1701"}
	SortKeys(keys)
	if strings.Join(keys, ",") != "P2,P47,P1701,mac,prog" {
		t.Fatalf("%v", keys)
	}
}
