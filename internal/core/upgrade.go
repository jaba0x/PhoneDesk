package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	pathpkg "path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jaba0x/phonedesk/internal/firmware"
	"github.com/jaba0x/phonedesk/internal/grandstream"
	"github.com/jaba0x/phonedesk/internal/relay"
	"github.com/jaba0x/phonedesk/internal/store"
)

// Job is one firmware update of one phone. Phones are updated one after
// another so a bad image doesn't take a whole office down at once.
type Job struct {
	ID        string    `json:"id"`
	PhoneID   string    `json:"phoneId"`
	PhoneName string    `json:"phoneName"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Image     string    `json:"image"`
	Family    string    `json:"family"`
	Source    string    `json:"source"` // where the phone fetched from
	State     string    `json:"state"`  // queued, running, done, failed, cancelled
	Step      string    `json:"step"`
	Progress  float64   `json:"progress"`
	Log       []string  `json:"log"`
	Error     string    `json:"error"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt"`
}

type upgrader struct {
	mu      sync.Mutex
	jobs    []*Job
	queue   chan *Job
	fetches map[string]chan struct{} // server token -> closed when the phone fetched the image
	relay   map[string]RelayState    // library file -> copy state on the relay server
}

func (u *upgrader) jobFor(phoneID string) *Job {
	u.mu.Lock()
	defer u.mu.Unlock()
	var latest *Job
	for _, j := range u.jobs {
		if j.PhoneID == phoneID {
			latest = j
		}
	}
	if latest == nil {
		return nil
	}
	cp := *latest
	return &cp
}

// fetchedToken is called by the HTTP server when a phone downloads an image.
func (u *upgrader) fetched(token, remote string, size int64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if ch, ok := u.fetches[token]; ok {
		select {
		case <-ch:
		default:
			close(ch)
		}
	}
	for _, j := range u.jobs {
		if j.State == "running" && strings.HasSuffix(j.Source, "/"+token) {
			j.Log = append(j.Log, fmt.Sprintf("%s  %s downloaded the image (%d MB)", time.Now().Format("15:04:05"), remote, size>>20))
		}
	}
}

// Jobs lists jobs, newest first.
func (m *Manager) Jobs() []Job {
	m.up.mu.Lock()
	defer m.up.mu.Unlock()
	out := make([]Job, 0, len(m.up.jobs))
	for i := len(m.up.jobs) - 1; i >= 0; i-- {
		out = append(out, *m.up.jobs[i])
	}
	return out
}

func (m *Manager) CancelJob(id string) error {
	m.up.mu.Lock()
	defer m.up.mu.Unlock()
	for _, j := range m.up.jobs {
		if j.ID == id {
			if j.State != "queued" {
				return errors.New("only queued updates can be cancelled")
			}
			j.State, j.Step, j.EndedAt = "cancelled", "Cancelled", time.Now()
			m.changed()
			return nil
		}
	}
	return errors.New("job not found")
}

func (m *Manager) ClearFinishedJobs() {
	m.up.mu.Lock()
	keep := m.up.jobs[:0]
	for _, j := range m.up.jobs {
		if j.State == "queued" || j.State == "running" {
			keep = append(keep, j)
		}
	}
	m.up.jobs = keep
	m.up.mu.Unlock()
	m.changed()
}

// StartUpgrade queues updates. image "" picks the newest image for each
// phone's family.
func (m *Manager) StartUpgrade(ids []string, image string) ([]Job, error) {
	if m.lib == nil {
		return nil, errors.New("the firmware library couldn't be opened")
	}
	var out []Job
	var errs []string
	for _, id := range ids {
		p, ok := m.st.Phone(id)
		if !ok {
			continue
		}
		m.mu.Lock()
		snap := m.snaps[id]
		m.mu.Unlock()
		name := displayName(p, snap)
		if p.Disabled {
			errs = append(errs, name+" is disabled")
			continue
		}
		fs := m.firmwareState(p, snap)
		if !fs.Supported {
			errs = append(errs, name+" hasn't been read yet")
			continue
		}
		var img firmware.Image
		if image != "" {
			img, ok = m.lib.Get(image)
			if !ok {
				errs = append(errs, image+" isn't in the library")
				continue
			}
			if img.Family != fs.Family {
				errs = append(errs, fmt.Sprintf("%s is for %s phones, %s is %s", image, img.Family, name, fs.Family))
				continue
			}
		} else {
			img, ok = m.lib.Latest(fs.Family)
			if !ok {
				errs = append(errs, "no image for "+fs.Family+" in the library ("+name+")")
				continue
			}
		}
		if j := m.up.jobFor(id); j != nil && (j.State == "queued" || j.State == "running") {
			errs = append(errs, name+" already has an update in progress")
			continue
		}
		job := &Job{ID: store.NewID(), PhoneID: id, PhoneName: name, From: fs.Running, To: img.Version.Text, Image: img.File, Family: img.Family, State: "queued", Step: "Waiting for its turn"}
		m.up.mu.Lock()
		m.up.jobs = append(m.up.jobs, job)
		m.up.mu.Unlock()
		select {
		case m.up.queue <- job:
			out = append(out, *job)
		default:
			errs = append(errs, "too many updates queued")
		}
	}
	m.changed()
	if len(out) == 0 && len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}
	if len(errs) > 0 {
		return out, errors.New(strings.Join(errs, "; "))
	}
	return out, nil
}

func (m *Manager) jobWorker() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case j := <-m.up.queue:
			m.up.mu.Lock()
			skip := j.State != "queued"
			if !skip {
				j.State, j.StartedAt = "running", time.Now()
			}
			m.up.mu.Unlock()
			if skip {
				continue
			}
			m.runJob(j)
		}
	}
}

func (m *Manager) jobLog(j *Job, step string, progress float64, f string, a ...any) {
	m.up.mu.Lock()
	if step != "" {
		j.Step = step
	}
	if progress >= 0 {
		j.Progress = progress
	}
	if f != "" {
		j.Log = append(j.Log, time.Now().Format("15:04:05")+"  "+fmt.Sprintf(f, a...))
	}
	m.up.mu.Unlock()
	m.changed()
}

func (m *Manager) finishJob(j *Job, err error) {
	m.up.mu.Lock()
	j.EndedAt = time.Now()
	if err != nil {
		j.State, j.Error, j.Step = "failed", err.Error(), "Failed"
		j.Log = append(j.Log, time.Now().Format("15:04:05")+"  Failed: "+err.Error())
	} else {
		j.State, j.Step, j.Progress = "done", "Done", 1
	}
	m.up.mu.Unlock()
	m.changed()
}

// runJob does one update: point the phone at the image, reboot it, wait for
// it to come back and check the version it runs.
func (m *Manager) runJob(j *Job) {
	p, ok := m.st.Phone(j.PhoneID)
	if !ok {
		m.finishJob(j, errors.New("phone was deleted"))
		return
	}
	l := m.lock(p.ID)
	l.Lock()
	defer l.Unlock()
	ctx, cancel := context.WithTimeout(m.ctx, 20*time.Minute)
	defer cancel()
	set := m.st.Settings()

	// where the phone fetches from
	path, via, token, err := m.firmwareSource(ctx, set, p, j)
	if err != nil {
		m.finishJob(j, err)
		return
	}
	if token != "" {
		defer m.srv.Withdraw(token)
		m.up.mu.Lock()
		if m.up.fetches == nil {
			m.up.fetches = map[string]chan struct{}{}
		}
		m.up.fetches[token] = make(chan struct{})
		fetchedCh := m.up.fetches[token]
		m.up.mu.Unlock()
		defer func() {
			m.up.mu.Lock()
			delete(m.up.fetches, token)
			m.up.mu.Unlock()
		}()
		_ = fetchedCh
	}
	m.up.mu.Lock()
	j.Source = path
	m.up.mu.Unlock()

	m.jobLog(j, "Logging in", 0.05, "Updating %s from %s to %s (%s)", j.PhoneName, j.From, j.To, j.Image)
	c, _, err := m.client(p)
	if err != nil {
		m.finishJob(j, err)
		return
	}
	if err := m.login(ctx, p, c); err != nil {
		m.finishJob(j, err)
		return
	}
	keys := []string{grandstream.PFirmwarePath, grandstream.PFirmwareVia, grandstream.PAlwaysCheck, grandstream.PFirmwarePre, grandstream.PFirmwareSuf}
	before, err := c.Values(ctx, keys)
	if err != nil {
		c.Logout(context.Background())
		m.finishJob(j, fmt.Errorf("reading upgrade settings: %w", err))
		return
	}
	m.jobLog(j, "Pointing the phone at the image", 0.15, "Upgrade settings before: server %q via %s, check %q", before[grandstream.PFirmwarePath], grandstream.Display(grandstream.PFirmwareVia, before[grandstream.PFirmwareVia]), grandstream.Display(grandstream.PAlwaysCheck, before[grandstream.PAlwaysCheck]))
	want := map[string]string{grandstream.PFirmwarePath: path, grandstream.PFirmwareVia: via, grandstream.PAlwaysCheck: "0", grandstream.PFirmwarePre: "", grandstream.PFirmwareSuf: ""}
	if err := c.SetValues(ctx, want); err != nil {
		c.Logout(context.Background())
		m.finishJob(j, fmt.Errorf("setting the firmware server: %w", err))
		return
	}
	m.jobLog(j, "", 0.25, "Firmware server set to %s (%s), always check for new firmware", path, grandstream.Display(grandstream.PFirmwareVia, via))
	m.jobLog(j, "Rebooting", 0.3, "Rebooting; the phone fetches %s on the way up", grandstream.FirmwareFile(j.Family))
	rebootUnsure := false
	if err := c.Reboot(ctx); err != nil {
		if !grandstream.NoAnswer(err) {
			c.Logout(context.Background())
			m.finishJob(j, fmt.Errorf("reboot refused: %w (turn on Action URI support under Settings → Remote control on the phone, or reboot it by hand)", err))
			return
		}
		// GXP16xx 1.0.2.x reboots as soon as it reads the request and never
		// answers it; whether it went down tells us if the reboot took
		rebootUnsure = true
		m.jobLog(j, "", 0.32, "No answer to the reboot request (%v); some firmware reboots before replying, so watching whether the phone goes down", err)
	} else {
		c.Logout(context.Background())
	}

	// down, then up again
	port := webPorts(p, c.UsedScheme())[0]
	m.jobLog(j, "Waiting for the phone to go down", 0.35, "")
	if !waitPort(ctx, p.Host, port, false, 90*time.Second) {
		if rebootUnsure {
			m.finishJob(j, errors.New("the phone didn't answer the reboot request and is still up after 90 s (turn on Action URI support under Settings → Remote control on the phone, or reboot it by hand)"))
			return
		}
		m.jobLog(j, "", 0.4, "The phone is still answering after 90 s; it may be rebooting slowly or ignoring the request")
	}
	if token != "" {
		m.jobLog(j, "Waiting for the phone to download the image", 0.45, "")
		m.up.mu.Lock()
		ch := m.up.fetches[token]
		m.up.mu.Unlock()
		select {
		case <-ch:
			m.jobLog(j, "Installing", 0.65, "Image downloaded; the phone is writing it")
		case <-time.After(4 * time.Minute):
			m.jobLog(j, "", 0.5, "No download from the phone after 4 min. It may not reach this computer on port %d (firewall, VPN), or it decided the image isn't newer", m.srv.Port())
		case <-ctx.Done():
			m.finishJob(j, ctx.Err())
			return
		}
	} else {
		m.jobLog(j, "Installing", 0.55, "Waiting for the phone to come back")
	}
	m.jobLog(j, "Waiting for the phone to come back", 0.75, "")
	if !waitPort(ctx, p.Host, port, true, 10*time.Minute) {
		m.finishJob(j, errors.New("the phone didn't come back within 10 minutes"))
		return
	}
	time.Sleep(10 * time.Second) // let the web server settle after boot
	m.jobLog(j, "Checking the version", 0.85, "The phone is back")
	c2, _, err := m.client(p)
	if err != nil {
		m.finishJob(j, err)
		return
	}
	var running string
	for attempt := 0; attempt < 6; attempt++ {
		if err = m.login(ctx, p, c2); err == nil {
			v, verr := c2.Values(ctx, []string{"prog", "prog_version"})
			if verr == nil {
				running = v["prog"]
				if running == "" {
					running = v["prog_version"]
				}
				break
			}
			err = verr
			c2.Logout(context.Background())
		}
		time.Sleep(10 * time.Second)
	}
	if err != nil {
		m.finishJob(j, fmt.Errorf("the phone is back but can't be read: %w", err))
		return
	}
	defer c2.Logout(context.Background())
	// put the upgrade settings back the way they were
	restore := map[string]string{}
	for _, k := range keys {
		if v, ok := before[k]; ok && v != want[k] {
			restore[k] = v
		}
	}
	if len(restore) > 0 {
		if err := c2.SetValues(ctx, restore); err != nil {
			m.jobLog(j, "", 0.9, "Couldn't restore the previous upgrade settings: %s", err.Error())
		} else {
			m.jobLog(j, "", 0.9, "Upgrade settings restored (server %q)", before[grandstream.PFirmwarePath])
		}
	}
	m.Enqueue(p.ID)
	cur, _ := parseVer(running)
	target, _ := parseVer(j.To)
	switch {
	case running == "":
		m.finishJob(j, errors.New("the phone didn't report its version"))
	case cur.Compare(target) == 0:
		m.jobLog(j, "", 0.95, "Running %s", running)
		m.finishJob(j, nil)
	case cur.Newer(target):
		m.jobLog(j, "", 0.95, "Running %s, which is newer than the image", running)
		m.finishJob(j, nil)
	default:
		m.finishJob(j, fmt.Errorf("the phone came back on %s, not %s; it didn't install the image", running, j.To))
	}
}

// viaCode turns "tftp", "http" or "https" into the phone's P-value.
func viaCode(via string) string {
	switch via {
	case "https":
		return "2"
	case "tftp":
		return "0"
	}
	return "1"
}

// relayOptions builds the SSH options for the relay server from the settings.
func (m *Manager) relayOptions(set store.Settings) (relay.Options, error) {
	pw, err := m.st.RelayPassword()
	if err != nil {
		return relay.Options{}, fmt.Errorf("can't read the relay password: %w", err)
	}
	return relay.Options{Host: set.RelayHost, Port: set.RelayPort, User: set.RelayUser, Password: pw, KeyFile: set.RelayKeyFile, KnownKey: set.RelayHostKey}, nil
}

// relayDial connects to the relay server and pins its host key on first use.
func (m *Manager) relayDial(ctx context.Context, set store.Settings) (*relay.Client, error) {
	o, err := m.relayOptions(set)
	if err != nil {
		return nil, err
	}
	c, err := relay.Dial(ctx, o)
	if err != nil {
		if errors.Is(err, relay.ErrHostKeyMismatch) {
			return nil, errors.New("the relay server's SSH host key changed since it was first used; if the server was reinstalled, use Forget host key in Settings")
		}
		return nil, err
	}
	if set.RelayHostKey == "" && c.Fingerprint != "" {
		m.st.SetRelayHostKey(c.Fingerprint)
	}
	return c, nil
}

// RelayState says whether an image has been copied to the relay server.
type RelayState struct {
	State   string    `json:"state"` // copying, synced, failed
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

func (m *Manager) setRelayState(file, state, msg string) {
	m.up.mu.Lock()
	if m.up.relay == nil {
		m.up.relay = map[string]RelayState{}
	}
	m.up.relay[file] = RelayState{State: state, Message: msg, At: time.Now()}
	m.up.mu.Unlock()
	m.changed()
}

// RelayStates returns the copy state of every image, by file name.
func (m *Manager) RelayStates() map[string]RelayState {
	m.up.mu.Lock()
	defer m.up.mu.Unlock()
	out := make(map[string]RelayState, len(m.up.relay))
	for k, v := range m.up.relay {
		out[k] = v
	}
	return out
}

// SyncRelay copies the given library images (all of them when files is
// empty) to the relay server, so an update doesn't have to. progress is
// called after each file.
func (m *Manager) SyncRelay(ctx context.Context, files []string, progress func(file, state, msg string)) error {
	set := m.st.Settings()
	if set.FirmwareSource != "relay" {
		return errors.New("the relay isn't the firmware source")
	}
	if m.lib == nil {
		return errors.New("the firmware library couldn't be opened")
	}
	if len(files) == 0 {
		for _, img := range m.lib.List() {
			files = append(files, img.File)
		}
	}
	if len(files) == 0 {
		return nil
	}
	report := func(file, state, msg string) {
		m.setRelayState(file, state, msg)
		if progress != nil {
			progress(file, state, msg)
		}
	}
	for _, f := range files {
		report(f, "copying", "")
	}
	c, err := m.relayDial(ctx, set)
	if err != nil {
		for _, f := range files {
			report(f, "failed", err.Error())
		}
		return err
	}
	defer c.Close()
	var firstErr error
	for _, f := range files {
		img, ok := m.lib.Get(f)
		if !ok {
			report(f, "failed", "not in the library")
			continue
		}
		local, err := m.lib.Path(f)
		if err != nil {
			report(f, "failed", err.Error())
			continue
		}
		dir := pathpkg.Join(set.RelayDir, "phonedesk", img.Version.Text)
		copied, err := c.Upload(ctx, local, dir, grandstream.FirmwareFile(img.Family))
		switch {
		case err != nil:
			report(f, "failed", err.Error())
			if firstErr == nil {
				firstErr = err
			}
		case copied:
			report(f, "synced", "copied to "+dir)
		default:
			report(f, "synced", "already on the server")
		}
	}
	return firstErr
}

// TestRelay connects to the relay server described by set (an empty password
// means the stored one) and checks the remote folder is writable.
func (m *Manager) TestRelay(set store.Settings) (string, error) {
	cur := m.st.Settings()
	o := relay.Options{Host: strings.TrimSpace(set.RelayHost), Port: set.RelayPort, User: strings.TrimSpace(set.RelayUser), Password: set.RelayPassword, KeyFile: strings.TrimSpace(set.RelayKeyFile)}
	if o.Password == "" {
		o.Password, _ = m.st.RelayPassword()
	}
	if o.Host == cur.RelayHost && o.Port == cur.RelayPort {
		o.KnownKey = cur.RelayHostKey
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := relay.Dial(ctx, o)
	if err != nil {
		if errors.Is(err, relay.ErrHostKeyMismatch) {
			return "", errors.New("the relay server's SSH host key changed since it was first used; if the server was reinstalled, use Forget host key below")
		}
		return "", err
	}
	defer c.Close()
	info, err := c.Check(strings.TrimSpace(set.RelayDir))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Connected to %s (%s). Host key %s", o.Host, info, relay.ShortFingerprint(c.Fingerprint)), nil
}

// firmwareSource works out the server path and transport for a job, copying
// the image to the relay server first when that's the source.
func (m *Manager) firmwareSource(ctx context.Context, set store.Settings, p store.Phone, j *Job) (path, via, token string, err error) {
	image := j.Image
	switch set.FirmwareSource {
	case "grandstream":
		return "firmware.grandstream.com", "1", "", nil
	case "custom":
		if set.FirmwareServer == "" {
			return "", "", "", errors.New("no custom firmware server set")
		}
		return set.FirmwareServer, viaCode(set.FirmwareVia), "", nil
	case "relay":
		if m.lib == nil {
			return "", "", "", errors.New("the firmware library couldn't be opened")
		}
		local, err := m.lib.Path(image)
		if err != nil {
			return "", "", "", err
		}
		name := grandstream.FirmwareFile(j.Family)
		dir := pathpkg.Join(set.RelayDir, "phonedesk", j.To)
		m.jobLog(j, "Copying the image to "+set.RelayHost, 0.03, "Uploading %s to %s/%s over SSH", image, dir, name)
		c, err := m.relayDial(ctx, set)
		if err != nil {
			return "", "", "", err
		}
		defer c.Close()
		copied, err := c.Upload(ctx, local, dir, name)
		if err != nil {
			m.setRelayState(image, "failed", err.Error())
			return "", "", "", err
		}
		if copied {
			m.jobLog(j, "", 0.04, "Copied %s to the relay server", name)
			m.setRelayState(image, "synced", "copied to "+dir)
		} else {
			m.jobLog(j, "", 0.04, "%s is already on the relay server", name)
			m.setRelayState(image, "synced", "already on the server")
		}
		return set.RelayServerPath + "/phonedesk/" + j.To, viaCode(set.RelayVia), "", nil
	}
	if m.srv == nil {
		return "", "", "", errors.New("the firmware server isn't available")
	}
	if err := m.srv.Start(set.FirmwarePort); err != nil {
		return "", "", "", fmt.Errorf("can't start the firmware server: %w", err)
	}
	ip, err := firmware.LocalIP(p.Host)
	if err != nil {
		return "", "", "", err
	}
	token = m.srv.Offer(image)
	return fmt.Sprintf("%s:%d/%s", ip, set.FirmwarePort, token), "1", token, nil
}

// waitPort waits until the TCP port is (or isn't) answering.
func waitPort(ctx context.Context, host, port string, up bool, max time.Duration) bool {
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return false
		}
		c, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
		if err == nil {
			c.Close()
		}
		if (err == nil) == up {
			return true
		}
		time.Sleep(3 * time.Second)
	}
	return false
}

func parseVer(s string) (firmware.Version, bool) { return firmware.ParseVersion(s) }

// FirmwareServerInfo tells the UI about the built-in server.
type FirmwareServerInfo struct {
	Running bool   `json:"running"`
	Port    int    `json:"port"`
	Example string `json:"example"`
}

func (m *Manager) FirmwareServerInfo() FirmwareServerInfo {
	if m.srv == nil {
		return FirmwareServerInfo{}
	}
	info := FirmwareServerInfo{Running: m.srv.Running(), Port: m.srv.Port()}
	if !info.Running {
		info.Port = m.st.Settings().FirmwarePort
	}
	if ip, err := firmware.LocalIP("8.8.8.8"); err == nil {
		info.Example = ip + ":" + strconv.Itoa(info.Port)
	}
	return info
}

// StartFirmwareServer turns the built-in server on outside of a job, for
// phones pointed at it by hand.
func (m *Manager) StartFirmwareServer() error {
	if m.srv == nil {
		return errors.New("the firmware library couldn't be opened")
	}
	return m.srv.Start(m.st.Settings().FirmwarePort)
}

func (m *Manager) StopFirmwareServer() {
	if m.srv != nil {
		m.srv.Stop()
	}
}
