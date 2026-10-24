package main

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	goruntime "runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/jaba0x/phonedesk/internal/core"
	"github.com/jaba0x/phonedesk/internal/grandstream"
	"github.com/jaba0x/phonedesk/internal/store"
)

// App is the object bound to the frontend; every exported method is callable
// from JavaScript as window.go.main.App.<Method>.
type App struct {
	ctx context.Context
	st  *store.Store
	mgr *core.Manager

	emitMu   sync.Mutex
	emitWait bool

	swapMu sync.Mutex // held while the data folder is being swapped
	notice string     // shown once in the UI after startup

	scanMu     sync.Mutex
	scanCancel context.CancelFunc
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	dir, _, notice, err := store.ResolveDir()
	a.notice = notice
	if err == nil {
		a.st, err = store.Open(dir)
	}
	if err != nil {
		runtime.MessageDialog(ctx, runtime.MessageDialogOptions{Type: runtime.ErrorDialog, Title: "PhoneDesk", Message: "Can't open data folder: " + err.Error()})
		os.Exit(1)
	}
	a.mgr = core.New(a.st, a.emit)
	a.mgr.Start()
	go a.updateLoop()
}

func (a *App) shutdown(ctx context.Context) {
	if a.mgr != nil {
		a.mgr.Stop()
	}
}

// emit coalesces bursts of change notifications into one UI refresh.
func (a *App) emit(string) {
	a.emitMu.Lock()
	if a.emitWait {
		a.emitMu.Unlock()
		return
	}
	a.emitWait = true
	a.emitMu.Unlock()
	time.AfterFunc(400*time.Millisecond, func() {
		a.emitMu.Lock()
		a.emitWait = false
		a.emitMu.Unlock()
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "changed")
		}
	})
}

func (a *App) Version() string  { return Version }
func (a *App) Platform() string { return goruntime.GOOS }

// ---- views ----

func (a *App) GetDashboard() core.Dashboard { return a.mgr.Dashboard() }

func (a *App) GetPhone(id string) (core.Detail, error) { return a.mgr.Detail(id) }

func (a *App) Extensions() []core.ExtensionRow { return a.mgr.Extensions() }

// ---- inventory ----

func (a *App) SavePhone(p store.Phone) (store.Phone, error) {
	isNew := p.ID == ""
	saved, err := a.st.SavePhone(p)
	if err != nil {
		return saved, err
	}
	a.mgr.ClearAuthFailure(saved.ID)
	if isNew {
		go a.mgr.Enqueue(saved.ID)
	} else {
		go a.mgr.Resume(saved.ID)
	}
	a.emit("")
	return saved, nil
}

type BulkResult struct {
	Added  int      `json:"added"`
	Errors []string `json:"errors"`
}

// BulkAdd takes one phone per line: an address, "name, address" or
// "address name", or a range like 10.0.1.20-10.0.1.60.
func (a *App) BulkAdd(text, credentialID, site, scheme string) BulkResult {
	res := BulkResult{}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, host := "", line
		if strings.Contains(line, ",") {
			parts := strings.SplitN(line, ",", 2)
			name, host = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			if net.ParseIP(name) != nil && net.ParseIP(host) == nil { // "ip,name"
				name, host = host, name
			}
		} else if f := strings.Fields(line); len(f) == 2 {
			host, name = f[0], f[1]
		}
		hosts := []string{host}
		if a1, _, ok := strings.Cut(host, "-"); ok && net.ParseIP(a1) != nil {
			expanded, err := core.ExpandTargets(host)
			if err != nil {
				res.Errors = append(res.Errors, err.Error())
				continue
			}
			hosts = expanded
			name = ""
		}
		for _, h := range hosts {
			_, err := a.st.SavePhone(store.Phone{Name: name, Host: h, CredentialID: credentialID, Site: site, Scheme: scheme})
			if err != nil {
				res.Errors = append(res.Errors, h+": "+err.Error())
				continue
			}
			res.Added++
		}
	}
	if res.Added > 0 {
		go a.mgr.RefreshAll()
		a.emit("")
	}
	return res
}

func (a *App) DeletePhone(id string) error {
	return a.DeletePhones([]string{id})
}

// DeletePhones removes several phones and their saved data.
func (a *App) DeletePhones(ids []string) error {
	for _, id := range ids {
		if err := a.st.DeletePhone(id); err != nil {
			return err
		}
		a.mgr.Forget(id)
	}
	a.emit("")
	return nil
}

// SetDisabled pauses (no polling, no reads, no alerts) or resumes phones.
func (a *App) SetDisabled(ids []string, disabled bool) error {
	if err := a.st.SetDisabled(ids, disabled); err != nil {
		return err
	}
	if !disabled {
		go func() {
			for _, id := range ids {
				a.mgr.Resume(id)
			}
		}()
	}
	a.emit("")
	return nil
}

func (a *App) TestPhone(p store.Phone) (string, error) { return a.mgr.TestConnection(p) }

func (a *App) Refresh(id string) { a.mgr.Enqueue(id) }

func (a *App) RefreshAll() { a.mgr.RefreshAll() }

// ---- discovery ----

// ScanNetwork looks for Grandstream phones in the given ranges and reports
// progress with "scan" events. Only one scan runs at a time.
func (a *App) ScanNetwork(text string) ([]core.Found, error) {
	hosts, err := core.ExpandTargets(text)
	if err != nil {
		return nil, err
	}
	if len(hosts) == 0 {
		return nil, errors.New("enter a network like 10.0.1.0/24 or a range like 10.0.1.20-60")
	}
	a.scanMu.Lock()
	if a.scanCancel != nil {
		a.scanMu.Unlock()
		return nil, errors.New("a scan is already running")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.scanCancel = cancel
	a.scanMu.Unlock()
	defer func() {
		a.scanMu.Lock()
		a.scanCancel = nil
		a.scanMu.Unlock()
		cancel()
	}()
	found := a.mgr.Scan(ctx, hosts, func(p core.ScanProgress) {
		runtime.EventsEmit(a.ctx, "scan", p)
	})
	return found, nil
}

func (a *App) CancelScan() {
	a.scanMu.Lock()
	defer a.scanMu.Unlock()
	if a.scanCancel != nil {
		a.scanCancel()
	}
}

// AddFound adds phones from a scan.
func (a *App) AddFound(hosts []string, credentialID, site string) BulkResult {
	return a.BulkAdd(strings.Join(hosts, "\n"), credentialID, site, "auto")
}

// ---- credentials & settings ----

func (a *App) Credentials() []store.Credential { return a.st.Credentials() }

func (a *App) SaveCredential(c store.Credential) (store.Credential, error) {
	out, err := a.st.SaveCredential(c)
	if err == nil {
		for _, p := range a.st.Phones() {
			if p.CredentialID == out.ID {
				a.mgr.ClearAuthFailure(p.ID)
			}
		}
	}
	return out, err
}

func (a *App) DeleteCredential(id string) error { return a.st.DeleteCredential(id) }

func (a *App) GetSettings() store.Settings { return a.st.Settings() }

func (a *App) SaveSettings(s store.Settings) error {
	off := []string{}
	for _, id := range s.ChecksOff {
		if core.KnownCheck(id) {
			off = append(off, id)
		}
	}
	s.ChecksOff = off
	err := a.st.SaveSettings(s)
	a.emit("")
	return err
}

// Checks lists the dashboard checks, for the switches in Settings.
func (a *App) Checks() []core.Check { return core.Checks }

// TestRelay tries the relay server with the settings as typed (an empty
// password means the stored one).
func (a *App) TestRelay(s store.Settings) (string, error) { return a.mgr.TestRelay(s) }

// ForgetRelayHostKey drops the pinned SSH host key, for a reinstalled relay server.
func (a *App) ForgetRelayHostKey() error { return a.st.SetRelayHostKey("") }

// PickRelayKeyFile chooses a private key file for the relay server.
func (a *App) PickRelayKeyFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Choose an SSH private key"})
}

// ---- actions on a phone ----

func (a *App) Reboot(id string) error { return a.mgr.Reboot(id) }

// Dial places a call from the phone.
func (a *App) Dial(id, number string, account int) error { return a.mgr.Dial(id, number, account) }

// GetValues reads keys from the phone right now (the console).
func (a *App) GetValues(id string, keys string) (map[string]string, error) {
	var list []string
	for _, k := range strings.FieldsFunc(keys, func(r rune) bool { return r == ' ' || r == ',' || r == ':' || r == '\n' }) {
		if k != "" {
			list = append(list, k)
		}
	}
	if len(list) == 0 {
		return nil, errors.New("enter one or more keys, like P47 P35")
	}
	return a.mgr.GetValues(id, list)
}

// PreviewValues shows what writing the lines would change on the phones.
func (a *App) PreviewValues(ids []string, lines string) ([]core.Preview, error) {
	values, err := grandstream.ParseLines(lines)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, errors.New("nothing to write")
	}
	return a.mgr.Preview(ids, values), nil
}

// ApplyValues writes the lines to the phones.
func (a *App) ApplyValues(ids []string, lines string, reboot bool) ([]core.ApplyResult, error) {
	values, err := grandstream.ParseLines(lines)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, errors.New("nothing to write")
	}
	return a.mgr.Apply(ids, values, reboot), nil
}

// PhoneValuesText returns a phone's settings as template lines.
func (a *App) PhoneValuesText(id string) (string, error) {
	v, err := a.mgr.PhoneValues(id)
	if err != nil {
		return "", err
	}
	return grandstream.FormatLines(v), nil
}

func (a *App) GetBackup(id, name string) (string, error) { return a.st.ReadBackup(id, name) }

// ---- templates ----

func (a *App) Templates() []grandstream.Template { return a.st.Templates() }

func (a *App) SaveTemplate(t grandstream.Template, lines string) (grandstream.Template, error) {
	values, err := grandstream.ParseLines(lines)
	if err != nil {
		return t, err
	}
	t.Values = values
	saved, err := a.st.SaveTemplate(t)
	a.emit("")
	return saved, err
}

func (a *App) DeleteTemplate(id string) error {
	err := a.st.DeleteTemplate(id)
	a.emit("")
	return err
}

// KnownParams lists the settings PhoneDesk knows by name, for the editor.
func (a *App) KnownParams() interface{} { return grandstream.GroupedParams() }

// TemplateLines renders a template's values as editable lines.
func (a *App) TemplateLines(id string) (string, error) {
	t, ok := a.st.Template(id)
	if !ok {
		return "", errors.New("template not found")
	}
	return grandstream.FormatLines(t.Values), nil
}

// ExportProvisioning writes cfg<mac>.xml files for the phones: one file
// when it's one phone, otherwise a zip. Returns the path written.
func (a *App) ExportProvisioning(ids []string, lines string) (string, error) {
	values, err := grandstream.ParseLines(lines)
	if err != nil {
		return "", err
	}
	files, missing := a.mgr.ProvisioningFiles(ids, values)
	if len(files) == 0 {
		if len(missing) > 0 {
			return "", fmt.Errorf("no MAC address known yet for %s; read the phone first", strings.Join(missing, ", "))
		}
		return "", errors.New("no phones chosen")
	}
	if len(files) == 1 {
		for name, b := range files {
			path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{DefaultFilename: name, Title: "Save provisioning file"})
			if err != nil {
				return "", err
			}
			if path == "" {
				return "", errors.New("cancelled")
			}
			return path, os.WriteFile(path, b, 0o644)
		}
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{DefaultFilename: "phonedesk-provisioning.zip", Title: "Save provisioning files"})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", errors.New("cancelled")
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	zw := zip.NewWriter(f)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		w, err := zw.Create(n)
		if err == nil {
			_, err = w.Write(files[n])
		}
		if err != nil {
			zw.Close()
			f.Close()
			return "", err
		}
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if len(missing) > 0 {
		return path, fmt.Errorf("saved %d files; skipped %s (no MAC known yet)", len(files), strings.Join(missing, ", "))
	}
	return path, nil
}

// ExportText asks where to save and writes the content (copies, raw output).
func (a *App) ExportText(defaultName, content string) (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{DefaultFilename: defaultName, Title: "Save"})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", errors.New("cancelled")
	}
	return path, os.WriteFile(path, []byte(content), 0o644)
}

func (a *App) OpenURL(u string) {
	if strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
		runtime.BrowserOpenURL(a.ctx, u)
	}
}

// OpenPhoneWeb opens the phone's own web UI in the browser.
func (a *App) OpenPhoneWeb(id string) error {
	p, ok := a.st.Phone(id)
	if !ok {
		return errors.New("phone not found")
	}
	scheme := p.Scheme
	if scheme == "auto" {
		scheme = "https"
		if snap := a.mgr.Snapshot(id); snap != nil && snap.Scheme != "" {
			scheme = snap.Scheme
		}
	}
	host := p.Host
	if p.Port != 0 {
		host = net.JoinHostPort(p.Host, fmt.Sprint(p.Port))
	}
	runtime.BrowserOpenURL(a.ctx, scheme+"://"+host+"/")
	return nil
}
