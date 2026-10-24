package grandstream

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client is one phone's web API: log in once, then read and write values.
// Phones ship with a self-signed certificate, so HTTPS is used without
// checking it; the phone is on your own network and the alternative is
// plain HTTP.
type Client struct {
	Host     string
	Port     int    // 0 = 443 for https, 80 for http
	Scheme   string // "auto" (https, then http), "https" or "http"
	Username string
	Password string
	Timeout  time.Duration

	mu        sync.Mutex
	sid       string
	scheme    string // the one that worked
	http      *http.Client
	raw       map[string]string // request -> response, for the Raw tab
	cookieSID string            // session-identity cookie from the last answer, if any
}

// legacyCipherSuites is Go's default list plus the ones it dropped (3DES, RC4,
// SHA-1 CBC), which some phones still need.
func legacyCipherSuites() []uint16 {
	var out []uint16
	for _, cs := range tls.CipherSuites() {
		out = append(out, cs.ID)
	}
	for _, cs := range tls.InsecureCipherSuites() {
		out = append(out, cs.ID)
	}
	return out
}

var (
	ErrAuth        = errors.New("login refused: wrong username or password")
	ErrUnreachable = errors.New("the phone doesn't answer on its web port")
	ErrNotGS       = errors.New("this doesn't look like a Grandstream phone")
)

// apiError is what the phone sent back when response != "success".
type apiError struct{ Body string }

func (e apiError) Error() string { return "phone answered: " + e.Body }

func (c *Client) client() *http.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.http == nil {
		t := c.Timeout
		if t == 0 {
			t = 12 * time.Second
		}
		c.http = &http.Client{
			Timeout: t,
			Transport: &http.Transport{
				// Self-signed certificates, and on older firmware TLS 1.0/1.1 with
				// ciphers Go no longer offers by default. The phone is on your own
				// network and the alternative is plain HTTP, so accept them.
				TLSClientConfig:     &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS10, CipherSuites: legacyCipherSuites()}, //nolint:gosec // see above
				DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
				MaxIdleConnsPerHost: 2,
				DisableKeepAlives:   true,
			},
			// the phone redirects http -> https on some firmware; follow but keep the method
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) > 3 {
					return errors.New("too many redirects")
				}
				return nil
			},
		}
	}
	return c.http
}

// Scheme reports which scheme the last successful login used.
func (c *Client) UsedScheme() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.scheme
}

// Raw returns the recorded request/response pairs (newest login onwards).
func (c *Client) Raw() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]string{}
	for k, v := range c.raw {
		out[k] = v
	}
	return out
}

func (c *Client) record(name, body string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.raw == nil {
		c.raw = map[string]string{}
	}
	if len(body) > 200000 {
		body = body[:200000] + "\n… (cut)"
	}
	c.raw[name] = body
}

func (c *Client) base(scheme string) string {
	host := c.Host
	if c.Port != 0 && !((scheme == "https" && c.Port == 443) || (scheme == "http" && c.Port == 80)) {
		host = net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	}
	return scheme + "://" + host
}

func (c *Client) schemes() []string {
	switch c.Scheme {
	case "https":
		return []string{"https"}
	case "http":
		return []string{"http"}
	}
	return []string{"https", "http"}
}

// response is the envelope every API call returns.
type response struct {
	Response string          `json:"response"`
	Body     json.RawMessage `json:"body"`
	Misc     json.RawMessage `json:"misc"`
}

func (c *Client) do(ctx context.Context, method, scheme, path string, form url.Values, withSession bool) (response, string, error) {
	u := c.base(scheme) + path
	var body io.Reader
	if method == "GET" && len(form) > 0 {
		u += "?" + form.Encode()
	} else if len(form) > 0 {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return response{}, "", err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("User-Agent", "PhoneDesk")
	req.Header.Set("Accept", "application/json, */*")
	// GXP16xx/21xx firmware refuses API calls that don't look like they come
	// from the phone's own web page (HTTP 403 "Forbidden"), so send what a
	// browser would.
	req.Header.Set("Referer", c.base(scheme)+"/")
	req.Header.Set("Origin", c.base(scheme))
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	if withSession && c.sid != "" {
		req.Header.Set("Cookie", "session-role=admin; session-identity="+c.sid)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return response{}, "", err
	}
	defer resp.Body.Close()
	for _, ck := range resp.Cookies() {
		if ck.Name == "session-identity" && ck.Value != "" {
			c.mu.Lock()
			c.cookieSID = ck.Value
			c.mu.Unlock()
		}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return response{}, "", err
	}
	text := string(b)
	var r response
	if jerr := json.Unmarshal(bytes.TrimPrefix(bytes.TrimSpace(b), []byte("\xef\xbb\xbf")), &r); jerr != nil {
		return response{}, text, fmt.Errorf("unexpected answer from %s (HTTP %d, %s)", path, resp.StatusCode, describeBody(text))
	}
	return r, text, nil
}

// Login opens a session. With Scheme "auto" it tries https, then http.
func (c *Client) Login(ctx context.Context) error {
	if c.Host == "" {
		return errors.New("no address")
	}
	user := c.Username
	if user == "" {
		user = "admin"
	}
	var last error
	for _, scheme := range c.schemes() {
		form := url.Values{"username": {user}, "password": {c.Password}}
		c.mu.Lock()
		c.cookieSID = ""
		c.mu.Unlock()
		r, text, err := c.do(ctx, "POST", scheme, "/cgi-bin/dologin", form, false)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) || isConnErr(err) {
				last = ErrUnreachable
				continue
			}
			if text != "" {
				c.record("dologin ("+scheme+") answer", text)
			}
			last = err
			continue
		}
		c.record("dologin ("+scheme+")", redactSID(text))
		if r.Response != "success" {
			// GRP answers "unauthorized"; GXP16xx/21xx answer "wrong" with the attempt count ("wrong3")
			lb := strings.ToLower(string(r.Body))
			if strings.Contains(lb, "unauthor") || strings.Contains(lb, "wrong") || strings.Contains(lb, "password") || strings.Contains(lb, "login") {
				return ErrAuth
			}
			return apiError{Body: strings.Trim(string(r.Body), `"`)}
		}
		var bd struct {
			SID  string `json:"sid"`
			Role string `json:"role"`
		}
		json.Unmarshal(r.Body, &bd) //nolint:errcheck // the body may also be a bare string
		if bd.SID == "" {
			// some firmware answers with the sid as a plain string, or only sets the cookie
			var plain string
			if json.Unmarshal(r.Body, &plain) == nil && plain != "" && plain != "success" {
				bd.SID = plain
			}
		}
		if bd.SID == "" {
			c.mu.Lock()
			bd.SID = c.cookieSID
			c.mu.Unlock()
		}
		if bd.SID == "" {
			return errors.New("the phone logged in but sent no session id (see dologin under Raw output); its firmware may be too old for the API")
		}
		c.mu.Lock()
		c.sid, c.scheme = bd.SID, scheme
		c.mu.Unlock()
		return nil
	}
	if last == nil {
		last = ErrUnreachable
	}
	return last
}

func isConnErr(err error) bool {
	s := err.Error()
	return strings.Contains(s, "refused") || strings.Contains(s, "timeout") || strings.Contains(s, "no route") || strings.Contains(s, "reset by peer") || strings.Contains(s, "EOF") || strings.Contains(s, "no such host")
}

func redactSID(s string) string {
	// keep the shape of the answer in the raw log, not the session token
	if i := strings.Index(s, `"sid"`); i >= 0 {
		j := strings.Index(s[i+5:], `"`)
		if j >= 0 {
			k := strings.Index(s[i+6+j:], `"`)
			if k >= 0 {
				return s[:i+6+j] + "…" + s[i+6+j+k:]
			}
		}
	}
	return s
}

func (c *Client) session() (string, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sid == "" {
		return "", "", errors.New("not logged in")
	}
	return c.sid, c.scheme, nil
}

// Values reads the given keys. Keys the phone doesn't know are simply absent
// from the result. Long lists are read in batches.
func (c *Client) Values(ctx context.Context, keys []string) (map[string]string, error) {
	sid, scheme, err := c.session()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	const batch = 50
	for i := 0; i < len(keys); i += batch {
		end := i + batch
		if end > len(keys) {
			end = len(keys)
		}
		part := keys[i:end]
		form := url.Values{"request": {strings.Join(part, ":")}, "sid": {sid}}
		r, text, err := c.do(ctx, "POST", scheme, "/cgi-bin/api.values.get", form, true)
		if err != nil {
			return out, err
		}
		c.record(fmt.Sprintf("api.values.get %s…%s", part[0], part[len(part)-1]), text)
		if r.Response != "success" {
			return out, apiError{Body: strings.Trim(string(r.Body), `"`)}
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(r.Body, &m); err != nil {
			return out, fmt.Errorf("api.values.get: %w", err)
		}
		for k, v := range m {
			out[k] = rawString(v)
		}
	}
	return out, nil
}

func rawString(v json.RawMessage) string {
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	return strings.Trim(string(v), `"`)
}

// SetValues writes P-values. The phone validates each one; it answers with an
// error for the whole batch if any is refused, so the caller gets no partial
// result to reason about. Values take effect on the phone right away for most
// settings; accounts re-register, network changes may need a reboot.
func (c *Client) SetValues(ctx context.Context, values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	sid, scheme, err := c.session()
	if err != nil {
		return err
	}
	form := url.Values{"sid": {sid}}
	keys := make([]string, 0, len(values))
	for k := range values {
		if !IsPKey(k) {
			return fmt.Errorf("%q is not a P-value", k)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		form.Set(k, values[k])
	}
	r, text, err := c.do(ctx, "POST", scheme, "/cgi-bin/api.values.post", form, true)
	if err != nil {
		return err
	}
	c.record("api.values.post "+strings.Join(keys, " "), text)
	if r.Response != "success" {
		return apiError{Body: strings.Trim(string(r.Body), `"`)}
	}
	return nil
}

// cti calls one of the remote-control endpoints. These accept the admin
// password as "passcode" and need Action URI support turned on in the phone's
// web UI (Settings → Remote control); the web session is sent along as well,
// which newer firmware accepts instead.
func (c *Client) cti(ctx context.Context, path string, params url.Values) (response, error) {
	sid, scheme, err := c.session()
	if err != nil {
		return response{}, err
	}
	if params == nil {
		params = url.Values{}
	}
	params.Set("passcode", c.Password)
	params.Set("sid", sid)
	r, text, err := c.do(ctx, "GET", scheme, path, params, true)
	if err != nil {
		return response{}, err
	}
	c.record(strings.TrimPrefix(path, "/cgi-bin/"), text)
	if r.Response != "success" {
		return r, apiError{Body: strings.Trim(string(r.Body), `"`)}
	}
	return r, nil
}

// Reboot restarts the phone.
func (c *Client) Reboot(ctx context.Context) error {
	_, err := c.cti(ctx, "/cgi-bin/api-sys_operation", url.Values{"request": {"REBOOT"}})
	return err
}

// FactoryReset wipes the phone. Needs the admin password.
func (c *Client) FactoryReset(ctx context.Context) error {
	_, err := c.cti(ctx, "/cgi-bin/api-sys_operation", url.Values{"request": {"RESET"}})
	return err
}

// Provision tells the phone to fetch its config now.
func (c *Client) Provision(ctx context.Context) error {
	_, err := c.cti(ctx, "/cgi-bin/api-sys_operation", url.Values{"request": {"PROV"}})
	return err
}

// MakeCall dials a number on an account (0-based, -1 = default).
func (c *Client) MakeCall(ctx context.Context, number string, account int) error {
	v := url.Values{"phonenumber": {number}}
	if account >= 0 {
		v.Set("account", strconv.Itoa(account))
	}
	_, err := c.cti(ctx, "/cgi-bin/api-make_call", v)
	return err
}

// EndCall hangs up a line.
func (c *Client) EndCall(ctx context.Context, line int) error {
	_, err := c.cti(ctx, "/cgi-bin/api-phone_operation", url.Values{"cmd": {"endcall"}, "line": {strconv.Itoa(line)}})
	return err
}

// SendKeys presses keys, e.g. "SPEAKER" or "1:0:0:0".
func (c *Client) SendKeys(ctx context.Context, keys string) error {
	_, err := c.cti(ctx, "/cgi-bin/api-send_key", url.Values{"keys": {keys}})
	return err
}

// PhoneStatus returns "available", "ringing", "busy" or "unauthorized".
func (c *Client) PhoneStatus(ctx context.Context) (string, error) {
	r, err := c.cti(ctx, "/cgi-bin/api-get_phone_status", nil)
	if err != nil {
		return "", err
	}
	return rawString(r.Body), nil
}

// Time reads the phone's clock and uptime from api-get_time, which GXP16xx
// and GXP21xx firmware uses for its own System Info page. Missing on some
// firmware; the caller treats an error as "no uptime".
func (c *Client) Time(ctx context.Context) (map[string]string, error) {
	sid, scheme, err := c.session()
	if err != nil {
		return nil, err
	}
	r, text, err := c.do(ctx, "GET", scheme, "/cgi-bin/api-get_time", url.Values{"sid": {sid}}, true)
	if err != nil {
		return nil, err
	}
	c.record("api-get_time", text)
	if r.Response != "success" {
		return nil, apiError{Body: strings.Trim(string(r.Body), `"`)}
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(r.Body, &m); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for k, v := range m {
		out[k] = rawString(v)
	}
	return out, nil
}

// LineStatus reads the live state of every line key.
func (c *Client) LineStatus(ctx context.Context) ([]Line, error) {
	r, err := c.cti(ctx, "/cgi-bin/api-get_line_status", nil)
	if err != nil {
		return nil, err
	}
	return parseLines(r.Body)
}

func parseLines(body json.RawMessage) ([]Line, error) {
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("line status: %w", err)
	}
	states := map[string]string{"0": "idle", "1": "dialing", "2": "ringing", "3": "calling", "4": "incall", "5": "hold", "8": "failed"}
	var out []Line
	for _, it := range items {
		l := Line{}
		l.Line, _ = strconv.Atoi(rawString(it["line"]))
		st := strings.ToLower(rawString(it["state"]))
		if s, ok := states[st]; ok {
			st = s
		}
		if st == "in call" || st == "connected" {
			st = "incall"
		}
		l.State = st
		l.Account, _ = strconv.Atoi(rawString(it["acct"]))
		l.Active = rawString(it["active"]) == "1" || rawString(it["active"]) == "true"
		l.RemoteName = rawString(it["remotename"])
		if l.RemoteName == "" {
			l.RemoteName = rawString(it["name"])
		}
		l.RemoteNumber = rawString(it["remotenumber"])
		if l.RemoteNumber == "" {
			l.RemoteNumber = rawString(it["number"])
		}
		out = append(out, l)
	}
	return out, nil
}

// Logout ends the session; phones keep only a few open.
func (c *Client) Logout(ctx context.Context) {
	sid, scheme, err := c.session()
	if err != nil {
		return
	}
	c.do(ctx, "GET", scheme, "/cgi-bin/logout", url.Values{"sid": {sid}}, true) //nolint:errcheck
	c.mu.Lock()
	c.sid = ""
	c.mu.Unlock()
}

// Fingerprint checks whether something at host answers like a Grandstream
// phone, without credentials. It returns the scheme that answered.
func Fingerprint(ctx context.Context, host string, timeout time.Duration) (string, error) {
	c := &Client{Host: host, Timeout: timeout}
	for _, scheme := range []string{"https", "http"} {
		// the API answers unauthenticated requests with an error envelope,
		// which is itself the fingerprint
		r, text, err := c.do(ctx, "GET", scheme, "/cgi-bin/api.values.get", url.Values{"request": {"vendor_name"}}, false)
		if err == nil && (r.Response == "success" || r.Response == "error") {
			return scheme, nil
		}
		if err != nil && strings.Contains(strings.ToLower(text), "grandstream") {
			return scheme, nil
		}
		if err != nil && isConnErr(err) && scheme == "https" {
			continue
		}
	}
	return "", ErrNotGS
}

// describeBody says in a few words what a non-JSON answer looked like.
func describeBody(text string) string {
	t := strings.TrimSpace(text)
	switch {
	case t == "":
		return "empty body"
	case strings.HasPrefix(strings.ToLower(t), "<!doctype") || strings.HasPrefix(t, "<html") || strings.Contains(t, "<body"):
		if strings.Contains(strings.ToLower(t), "login") {
			return "an HTML login page instead of JSON; this firmware may not have the API"
		}
		return "an HTML page instead of JSON"
	}
	if len(t) > 60 {
		t = t[:60] + "…"
	}
	return "body " + strconv.Quote(t)
}
