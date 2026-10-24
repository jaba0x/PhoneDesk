// Package core ties the inventory, phone reads, problem rules and update
// jobs together.
package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jaba0x/phonedesk/internal/firmware"
	"github.com/jaba0x/phonedesk/internal/grandstream"
	"github.com/jaba0x/phonedesk/internal/store"
)

type Status struct {
	Online       bool      `json:"online"`
	Checked      bool      `json:"checked"`
	LatencyMs    int       `json:"latencyMs"`
	PingError    string    `json:"pingError"`
	LastCheck    time.Time `json:"lastCheck"`
	Since        time.Time `json:"since"` // when Online last flipped
	Collecting   bool      `json:"collecting"`
	LastCollect  time.Time `json:"lastCollect"`
	CollectError string    `json:"collectError"`
	AuthFailed   bool      `json:"authFailed"`
	Scheme       string    `json:"scheme"` // scheme that worked last
}

// Manager runs the reachability checks and reads, and answers the UI.
type Manager struct {
	st     *store.Store
	notify func(string)

	mu     sync.Mutex
	status map[string]*Status
	snaps  map[string]*grandstream.Snapshot
	locks  map[string]*sync.Mutex
	active int

	lib *firmware.Library
	srv *firmware.Server
	up  upgrader

	queue chan string
	ctx   context.Context
	stop  context.CancelFunc

	// collectTimeout is how long one phone read may take
	collectTimeout time.Duration
}

func New(st *store.Store, notify func(string)) *Manager {
	m := &Manager{st: st, notify: notify, status: map[string]*Status{}, snaps: map[string]*grandstream.Snapshot{}, locks: map[string]*sync.Mutex{}, queue: make(chan string, 1024), collectTimeout: 60 * time.Second}
	m.up.queue = make(chan *Job, 512)
	if lib, err := firmware.OpenLibrary(filepath.Join(st.Dir(), "firmware")); err == nil {
		m.lib = lib
		m.srv = firmware.NewServer(lib, m.up.fetched)
	}
	for _, p := range st.Phones() {
		m.status[p.ID] = &Status{}
		if s, ok := st.LoadSnapshot(p.ID); ok {
			snap := s
			m.snaps[p.ID] = &snap
			m.status[p.ID].LastCollect = s.CollectedAt
			m.status[p.ID].Scheme = s.Scheme
		}
	}
	return m
}

func (m *Manager) changed() {
	if m.notify != nil {
		m.notify("changed")
	}
}

func (m *Manager) Library() *firmware.Library { return m.lib }

func (m *Manager) Start() {
	m.ctx, m.stop = context.WithCancel(context.Background())
	for i := 0; i < 16; i++ {
		go m.worker()
	}
	go m.pingLoop()
	go m.scheduleLoop()
	go m.jobWorker()
}

// Stop cancels polling and waits (up to 30s) for running reads, so the data
// folder can be swapped safely afterwards.
func (m *Manager) Stop() {
	if m.stop != nil {
		m.stop()
	}
	if m.srv != nil {
		m.srv.Stop()
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		n := m.active
		m.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (m *Manager) stat(id string) *Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.status[id]
	if !ok {
		s = &Status{}
		m.status[id] = s
	}
	return s
}

func (m *Manager) lock(id string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.locks[id]
	if !ok {
		l = &sync.Mutex{}
		m.locks[id] = l
	}
	return l
}

// ---------- reachability ----------

func (m *Manager) pingLoop() {
	m.pingAll()
	for {
		iv := time.Duration(m.st.Settings().PingInterval) * time.Second
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(iv):
			m.pingAll()
		}
	}
}

func (m *Manager) pingAll() {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 32)
	for _, p := range m.st.Phones() {
		if p.Disabled {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(p store.Phone) {
			defer wg.Done()
			defer func() { <-sem }()
			m.ping(p)
		}(p)
	}
	wg.Wait()
	m.changed()
}

// webPorts lists the TCP ports a phone's web UI may answer on, best guess first.
func webPorts(p store.Phone, scheme string) []string {
	if p.Port != 0 {
		return []string{strconv.Itoa(p.Port)}
	}
	switch {
	case p.Scheme == "http" || scheme == "http":
		return []string{"80", "443"}
	default:
		return []string{"443", "80"}
	}
}

func (m *Manager) ping(p store.Phone) {
	st := m.stat(p.ID)
	m.mu.Lock()
	scheme := st.Scheme
	m.mu.Unlock()
	start := time.Now()
	var err error
	for _, port := range webPorts(p, scheme) {
		var conn net.Conn
		conn, err = net.DialTimeout("tcp", net.JoinHostPort(p.Host, port), 3*time.Second)
		if err == nil {
			conn.Close()
			break
		}
	}
	lat := int(time.Since(start).Milliseconds())
	m.mu.Lock()
	online := err == nil
	if !st.Checked || st.Online != online {
		st.Since = time.Now()
	}
	wasOffline := st.Checked && !st.Online
	st.Online, st.Checked, st.LastCheck = online, true, time.Now()
	if online {
		st.LatencyMs, st.PingError = lat, ""
	} else {
		st.LatencyMs, st.PingError = 0, shortErr(err)
	}
	m.mu.Unlock()
	if online && wasOffline {
		m.Enqueue(p.ID)
	}
}

func shortErr(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "timeout"):
		return "timeout"
	case strings.Contains(s, "refused"):
		return "connection refused"
	case strings.Contains(s, "no route"):
		return "no route to host"
	case strings.Contains(s, "no such host"):
		return "unknown host"
	}
	if i := strings.LastIndex(s, ": "); i >= 0 {
		return s[i+2:]
	}
	return s
}

// ---------- reads ----------

func (m *Manager) scheduleLoop() {
	select {
	case <-m.ctx.Done():
		return
	case <-time.After(3 * time.Second):
	}
	for {
		set := m.st.Settings()
		if set.CollectInterval > 0 {
			due := time.Duration(set.CollectInterval) * time.Minute
			for _, p := range m.st.Phones() {
				st := m.stat(p.ID)
				m.mu.Lock()
				ready := !p.Disabled && st.Online && !st.Collecting && time.Since(st.LastCollect) > due && !st.AuthFailed
				m.mu.Unlock()
				if ready {
					m.Enqueue(p.ID)
				}
			}
		}
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
	}
}

func (m *Manager) Enqueue(id string) {
	if p, ok := m.st.Phone(id); !ok || p.Disabled {
		return
	}
	st := m.stat(id)
	m.mu.Lock()
	if st.Collecting {
		m.mu.Unlock()
		return
	}
	st.Collecting = true
	m.mu.Unlock()
	m.changed()
	select {
	case m.queue <- id:
	default:
		m.mu.Lock()
		st.Collecting = false
		m.mu.Unlock()
	}
}

func (m *Manager) RefreshAll() {
	for _, p := range m.st.Phones() {
		st := m.stat(p.ID)
		m.mu.Lock()
		ok := !p.Disabled && (st.Online || !st.Checked)
		m.mu.Unlock()
		if ok {
			m.Enqueue(p.ID)
		}
	}
}

func (m *Manager) worker() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case id := <-m.queue:
			m.collect(id)
		}
	}
}

// ClearAuthFailure lets a phone be read again after its credentials changed.
func (m *Manager) ClearAuthFailure(id string) {
	st := m.stat(id)
	m.mu.Lock()
	st.AuthFailed = false
	st.CollectError = ""
	m.mu.Unlock()
}

// Forget drops everything held in memory for a deleted phone.
func (m *Manager) Forget(id string) {
	m.mu.Lock()
	delete(m.status, id)
	delete(m.snaps, id)
	delete(m.locks, id)
	m.mu.Unlock()
}

// Resume re-checks a phone that was just enabled.
func (m *Manager) Resume(id string) {
	if p, ok := m.st.Phone(id); ok {
		m.ping(p)
		m.Enqueue(id)
		m.changed()
	}
}

// client builds a logged-out client for a phone with its credentials.
func (m *Manager) client(p store.Phone) (*grandstream.Client, store.Credential, error) {
	cred, err := m.st.Credential(p.CredentialID)
	if err != nil {
		return nil, cred, err
	}
	st := m.stat(p.ID)
	m.mu.Lock()
	scheme := st.Scheme
	m.mu.Unlock()
	c := &grandstream.Client{Host: p.Host, Port: p.Port, Scheme: p.Scheme, Username: cred.Username, Password: cred.Password}
	if c.Scheme == "auto" && scheme != "" {
		// try what worked last first; fall back to the other on failure
		c.Scheme = scheme
	}
	return c, cred, nil
}

// login opens a session, retrying with the other scheme when the remembered
// one stopped answering (the phone's web mode may have been switched).
func (m *Manager) login(ctx context.Context, p store.Phone, c *grandstream.Client) error {
	err := c.Login(ctx)
	if err != nil && errors.Is(err, grandstream.ErrUnreachable) && p.Scheme == "auto" && c.Scheme != "auto" {
		c.Scheme = "auto"
		err = c.Login(ctx)
	}
	return err
}

func (m *Manager) collect(id string) {
	p, ok := m.st.Phone(id)
	st := m.stat(id)
	finish := func() {
		m.mu.Lock()
		st.Collecting = false
		m.mu.Unlock()
		m.changed()
	}
	if !ok || p.Disabled {
		finish()
		return
	}
	l := m.lock(id)
	l.Lock()
	defer l.Unlock()
	m.mu.Lock()
	m.active++
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.active--
		m.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(m.ctx, m.collectTimeout)
	defer cancel()
	snap, err := m.read(ctx, p)
	m.mu.Lock()
	if err != nil {
		st.CollectError = err.Error()
		st.AuthFailed = errors.Is(err, grandstream.ErrAuth)
		m.mu.Unlock()
		finish()
		return
	}
	st.CollectError, st.AuthFailed = "", false
	st.LastCollect = snap.CollectedAt
	st.Scheme = snap.Scheme
	st.Online, st.Checked = true, true
	m.snaps[id] = &snap
	m.mu.Unlock()
	m.st.SaveSnapshot(id, snap)
	if snap.Facts.MAC != "" {
		m.st.SetMAC(id, snap.Facts.MAC)
	}
	m.st.SaveBackup(id, snap.Values, m.st.Settings().KeepBackups)
	finish()
}

// read logs in and reads everything PhoneDesk shows.
func (m *Manager) read(ctx context.Context, p store.Phone) (grandstream.Snapshot, error) {
	start := time.Now()
	c, cred, err := m.client(p)
	if err != nil {
		return grandstream.Snapshot{}, err
	}
	if err := m.login(ctx, p, c); err != nil {
		return grandstream.Snapshot{}, err
	}
	defer c.Logout(context.Background())
	values, err := c.Values(ctx, grandstream.CollectKeys())
	if err != nil {
		return grandstream.Snapshot{}, fmt.Errorf("reading settings: %w", err)
	}
	var errs []string
	lines, lerr := c.LineStatus(ctx)
	if lerr != nil {
		errs = append(errs, "Line status not available: "+lerr.Error()+". Turn on Action URI support under Settings → Remote control on the phone to see registration state.")
	}
	snap := grandstream.Build(values, lines, lerr == nil)
	snap.CollectedAt = time.Now()
	snap.Scheme = c.UsedScheme()
	snap.Errors = errs
	applyPortProbes(&snap, p.Host)
	snap.Security.DefaultAdmin = cred.Username == "admin" && cred.Password == "admin"
	snap.Security.PlainWeb = snap.Scheme == "http"
	snap.Raw = c.Raw()
	snap.Duration = time.Since(start).Round(100 * time.Millisecond).String()
	return snap, nil
}

// TestConnection logs in with the given settings and reports what answered.
func (m *Manager) TestConnection(p store.Phone) (string, error) {
	cred, err := m.st.Credential(p.CredentialID)
	if err != nil {
		return "", err
	}
	c := &grandstream.Client{Host: p.Host, Port: p.Port, Scheme: p.Scheme, Username: cred.Username, Password: cred.Password}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := c.Login(ctx); err != nil {
		return "", err
	}
	defer c.Logout(context.Background())
	v, err := c.Values(ctx, []string{"Pphone_model", "phone_model", "product_model", "prog", "mac", "vendor_fullname"})
	if err != nil {
		return "", err
	}
	snap := grandstream.Build(v, nil, false)
	model := snap.Facts.Model
	if model == "" {
		model = "a Grandstream phone"
	}
	out := fmt.Sprintf("Logged in over %s to %s", strings.ToUpper(c.UsedScheme()), model)
	if snap.Facts.Firmware != "" {
		out += ", firmware " + snap.Facts.Firmware
	}
	if snap.Facts.MAC != "" {
		out += ", " + snap.Facts.MAC
	}
	return out, nil
}

// Reboot restarts a phone.
func (m *Manager) Reboot(id string) error {
	p, ok := m.st.Phone(id)
	if !ok {
		return errors.New("phone not found")
	}
	c, _, err := m.client(p)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := m.login(ctx, p, c); err != nil {
		return err
	}
	defer c.Logout(context.Background())
	return c.Reboot(ctx)
}

// Dial places a call from a phone. account is 0-based, -1 = default.
func (m *Manager) Dial(id, number string, account int) error {
	p, ok := m.st.Phone(id)
	if !ok {
		return errors.New("phone not found")
	}
	c, _, err := m.client(p)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := m.login(ctx, p, c); err != nil {
		return err
	}
	defer c.Logout(context.Background())
	return c.MakeCall(ctx, number, account)
}

// GetValues reads arbitrary keys from a phone (the console).
func (m *Manager) GetValues(id string, keys []string) (map[string]string, error) {
	p, ok := m.st.Phone(id)
	if !ok {
		return nil, errors.New("phone not found")
	}
	c, _, err := m.client(p)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := m.login(ctx, p, c); err != nil {
		return nil, err
	}
	defer c.Logout(context.Background())
	return c.Values(ctx, keys)
}
