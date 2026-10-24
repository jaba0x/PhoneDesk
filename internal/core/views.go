package core

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jaba0x/phonedesk/internal/grandstream"
	"github.com/jaba0x/phonedesk/internal/store"
)

// AccountState is the per-account summary shown as a "line key" LED.
type AccountState struct {
	Index      int    `json:"index"`
	Active     bool   `json:"active"`
	Registered int    `json:"registered"` // 1, 0, -1 unknown
	UserID     string `json:"userId"`
	Name       string `json:"name"`
	Server     string `json:"server"`
}

// Summary is the part of a snapshot the lists need.
type Summary struct {
	Model             string             `json:"model"`
	Family            string             `json:"family"`
	Firmware          string             `json:"firmware"`
	MAC               string             `json:"mac"`
	IP                string             `json:"ip"`
	Uptime            string             `json:"uptime"`
	Extension         string             `json:"extension"` // first active account's user ID
	Display           string             `json:"display"`
	Server            string             `json:"server"`
	Accounts          []AccountState     `json:"accounts"`
	ActiveAccounts    int                `json:"activeAccounts"`
	Registered        int                `json:"registered"`
	RegistrationKnown bool               `json:"registrationKnown"`
	Lines             []grandstream.Line `json:"lines"`
	InCall            bool               `json:"inCall"`
	CollectedAt       time.Time          `json:"collectedAt"`
	Scheme            string             `json:"scheme"`
}

// FirmwareState is what the lists show about a phone's firmware.
type FirmwareState struct {
	Supported bool   `json:"supported"`
	Family    string `json:"family"`
	Running   string `json:"running"`
	Available string `json:"available"` // newest newer version in the library, or ""
	Image     string `json:"image"`
	Job       *Job   `json:"job"`
}

// PhoneView is a phone as the UI sees it.
type PhoneView struct {
	store.Phone
	Status   Status        `json:"status"`
	Summary  *Summary      `json:"summary"`
	Issues   []Issue       `json:"issues"`
	Worst    string        `json:"worst"`
	Firmware FirmwareState `json:"firmware"`
}

type Dashboard struct {
	Total      int         `json:"total"`
	Online     int         `json:"online"`
	Offline    int         `json:"offline"`
	Unknown    int         `json:"unknown"`
	Disabled   int         `json:"disabled"`
	Critical   int         `json:"critical"`
	Warning    int         `json:"warning"`
	Info       int         `json:"info"`
	Collecting int         `json:"collecting"`
	Registered int         `json:"registered"`
	Accounts   int         `json:"accounts"`
	Issues     []Issue     `json:"issues"`
	Phones     []PhoneView `json:"phones"`
	Sites      []string    `json:"sites"`
}

func summarize(s *grandstream.Snapshot) *Summary {
	if s == nil {
		return nil
	}
	sum := &Summary{Model: s.Facts.Model, Family: s.Facts.Family, Firmware: s.Facts.Firmware, MAC: s.Facts.MAC, IP: s.Facts.IP, Uptime: s.Facts.Uptime,
		RegistrationKnown: s.RegistrationKnown, Lines: s.Lines, CollectedAt: s.CollectedAt, Scheme: s.Scheme}
	for _, a := range s.Accounts {
		sum.Accounts = append(sum.Accounts, AccountState{Index: a.Index, Active: a.Active, Registered: a.Registered, UserID: a.UserID, Name: a.Name, Server: a.Server})
		if !a.Active {
			continue
		}
		sum.ActiveAccounts++
		if a.Registered == 1 {
			sum.Registered++
		}
		if sum.Extension == "" && a.UserID != "" {
			sum.Extension, sum.Display, sum.Server = a.UserID, a.Display, a.Server
		}
	}
	for _, l := range s.Lines {
		if l.State == "incall" || l.State == "ringing" || l.State == "calling" || l.State == "hold" || l.State == "dialing" {
			sum.InCall = true
		}
	}
	return sum
}

func (m *Manager) view(p store.Phone, set store.Settings) PhoneView {
	m.mu.Lock()
	st := m.status[p.ID]
	if st == nil {
		st = &Status{}
	}
	stc := *st
	snap := m.snaps[p.ID]
	m.mu.Unlock()
	v := PhoneView{Phone: p, Status: stc, Summary: summarize(snap)}
	v.Firmware = m.firmwareState(p, snap)
	if !p.Disabled {
		v.Issues = evaluate(p, &stc, snap, set, v.Firmware)
		sortIssues(v.Issues)
		if len(v.Issues) > 0 {
			v.Worst = string(v.Issues[0].Severity)
		}
	}
	return v
}

func (m *Manager) Dashboard() Dashboard {
	set := m.st.Settings()
	d := Dashboard{Issues: []Issue{}, Phones: []PhoneView{}}
	names := map[string]string{}
	snaps := map[string]*grandstream.Snapshot{}
	sites := map[string]bool{}
	for _, p := range m.st.Phones() {
		v := m.view(p, set)
		d.Total++
		if p.Site != "" {
			sites[p.Site] = true
		}
		if p.Disabled {
			d.Disabled++
		} else {
			switch {
			case !v.Status.Checked:
				d.Unknown++
			case v.Status.Online:
				d.Online++
			default:
				d.Offline++
			}
			if v.Status.Collecting {
				d.Collecting++
			}
			m.mu.Lock()
			snaps[p.ID] = m.snaps[p.ID]
			m.mu.Unlock()
			names[p.ID] = displayName(p, snaps[p.ID])
			if v.Summary != nil {
				d.Accounts += v.Summary.ActiveAccounts
				d.Registered += v.Summary.Registered
			}
		}
		d.Issues = append(d.Issues, v.Issues...)
		d.Phones = append(d.Phones, v)
	}
	d.Issues = append(d.Issues, fleetIssues(snaps, names)...)
	sortIssues(d.Issues)
	for _, i := range d.Issues {
		switch i.Severity {
		case Critical:
			d.Critical++
		case Warning:
			d.Warning++
		default:
			d.Info++
		}
	}
	for s := range sites {
		d.Sites = append(d.Sites, s)
	}
	sort.Strings(d.Sites)
	return d
}

// Detail is the full phone page.
type Detail struct {
	PhoneView
	Snapshot *grandstream.Snapshot `json:"snapshot"`
	Backups  []store.Backup        `json:"backups"`
}

func (m *Manager) Detail(id string) (Detail, error) {
	p, ok := m.st.Phone(id)
	if !ok {
		return Detail{}, errors.New("phone not found")
	}
	v := m.view(p, m.st.Settings())
	m.mu.Lock()
	snap := m.snaps[id]
	m.mu.Unlock()
	return Detail{PhoneView: v, Snapshot: snap, Backups: m.st.Backups(id)}, nil
}

// Snapshot returns the last reading of a phone, if any.
func (m *Manager) Snapshot(id string) *grandstream.Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snaps[id]
}

// Extension overview: every registered extension across the fleet.
type ExtensionRow struct {
	PhoneID    string `json:"phoneId"`
	PhoneName  string `json:"phoneName"`
	Account    int    `json:"account"`
	UserID     string `json:"userId"`
	Display    string `json:"display"`
	Server     string `json:"server"`
	Registered int    `json:"registered"`
	Active     bool   `json:"active"`
}

func (m *Manager) Extensions() []ExtensionRow {
	var out []ExtensionRow
	for _, p := range m.st.Phones() {
		m.mu.Lock()
		snap := m.snaps[p.ID]
		m.mu.Unlock()
		if snap == nil {
			continue
		}
		name := displayName(p, snap)
		for _, a := range snap.Accounts {
			if a.UserID == "" && a.Server == "" {
				continue
			}
			out = append(out, ExtensionRow{PhoneID: p.ID, PhoneName: name, Account: a.Index, UserID: a.UserID, Display: a.Display, Server: a.Server, Registered: a.Registered, Active: a.Active})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UserID != out[j].UserID {
			return strings.ToLower(out[i].UserID) < strings.ToLower(out[j].UserID)
		}
		return out[i].PhoneName < out[j].PhoneName
	})
	return out
}

func (m *Manager) firmwareState(p store.Phone, snap *grandstream.Snapshot) FirmwareState {
	fs := FirmwareState{Job: m.up.jobFor(p.ID)}
	if snap == nil || snap.Facts.Firmware == "" {
		return fs
	}
	fs.Supported = true
	fs.Running = snap.Facts.Firmware
	fs.Family = p.Family
	if fs.Family == "" {
		fs.Family = snap.Facts.Family
	}
	if m.lib == nil || fs.Family == "" {
		return fs
	}
	if img, ok := m.lib.Latest(fs.Family); ok {
		if cur, ok := parseVer(fs.Running); !ok || img.Version.Newer(cur) {
			fs.Available, fs.Image = img.Version.Text, img.File
		}
	}
	return fs
}
