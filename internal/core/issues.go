package core

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jaba0x/phonedesk/internal/grandstream"
	"github.com/jaba0x/phonedesk/internal/store"
)

type Severity string

const (
	Critical Severity = "critical"
	Warning  Severity = "warning"
	Info     Severity = "info"
)

func (s Severity) rank() int {
	switch s {
	case Critical:
		return 0
	case Warning:
		return 1
	}
	return 2
}

type Issue struct {
	PhoneID   string   `json:"phoneId"`
	PhoneName string   `json:"phoneName"`
	Severity  Severity `json:"severity"`
	Category  string   `json:"category"`
	Title     string   `json:"title"`
	Detail    string   `json:"detail"`
}

// evaluate turns one phone's status and last snapshot into issues.
func evaluate(p store.Phone, st *Status, snap *grandstream.Snapshot, set store.Settings, fw FirmwareState) []Issue {
	name := displayName(p, snap)
	var out []Issue
	add := func(sev Severity, cat, title, detail string) {
		out = append(out, Issue{PhoneID: p.ID, PhoneName: name, Severity: sev, Category: cat, Title: title, Detail: detail})
	}

	if st.Checked && !st.Online {
		d := "No answer on the web port"
		if !st.Since.IsZero() {
			d += " since " + st.Since.Format("Jan 2 15:04")
		}
		if st.PingError != "" {
			d += " (" + st.PingError + ")"
		}
		add(Critical, "Reachability", "Phone is offline", d)
	}
	if st.AuthFailed {
		add(Critical, "Login", "Web login failed", st.CollectError+". Fix the credentials, then read again.")
	} else if st.CollectError != "" && st.Online {
		add(Warning, "Login", "Reading the phone failed", st.CollectError)
	}
	if snap == nil || snap.CollectedAt.IsZero() {
		return out
	}

	// accounts
	active := 0
	for _, a := range snap.Accounts {
		if !a.Active {
			continue
		}
		active++
		label := fmt.Sprintf("Account %d", a.Index)
		if a.UserID != "" {
			label += " (" + a.UserID + ")"
		}
		if a.Server == "" {
			add(Warning, "Accounts", label+" has no SIP server", "The account is active but has nowhere to register.")
			continue
		}
		if a.Registered == 0 {
			add(Critical, "Accounts", label+" is not registered", "The phone can't register with "+a.Server+". Check the credentials on the PBX and the phone.")
		}
	}
	if active == 0 {
		add(Warning, "Accounts", "No active SIP account", "The phone has no account switched on, so it can't make or receive calls.")
	}

	// web access and management plane
	sec := snap.Security
	if sec.DefaultAdmin && set.FlagDefaultPassword {
		add(Critical, "Security", "Default admin password", "The phone still accepts admin/admin. Anyone on the network can reconfigure it. Set P2 from a template or on the phone, then update the credentials here.")
	}
	if sec.PlainWeb && set.FlagPlainHTTP {
		d := "PhoneDesk reached the web UI over plain HTTP; the admin password travels in clear text."
		if sec.HTTPSPort == "open" {
			d += " HTTPS answers too, so set the phone to HTTPS in its address here."
		} else {
			d += " Turn on HTTPS web access on the phone (Maintenance → Web/SSH access)."
		}
		add(Warning, "Security", "Web access over plain HTTP", d)
	}
	if sec.TelnetPort == "open" {
		add(Critical, "Security", "Telnet is open", "The phone accepts connections on TCP 23.")
	}
	if sec.SSHPort == "open" {
		add(Info, "Security", "SSH is open", "The phone accepts connections on TCP 22. Fine if you use it; otherwise switch it off under Maintenance → Web/SSH access.")
	}

	// time, logging, provisioning
	if snap.Time.NTPServer == "" && set.FlagNoNTP {
		add(Warning, "Settings", "No NTP server", "Without time sync the call history and log timestamps drift, and TLS registrations can fail.")
	}
	if snap.Logging.Server == "" && set.FlagNoSyslog {
		add(Info, "Settings", "No syslog server", "Events only live in the phone's own buffer until it reboots.")
	}
	if snap.Upgrade.Option66 != nil && *snap.Upgrade.Option66 && set.FlagOption66 {
		add(Info, "Provisioning", "DHCP option 66/43 may override the servers", "Any DHCP server on the phone's network can point it at another provisioning server.")
	}
	if fw.Supported && fw.Available != "" {
		add(Info, "Firmware", "Firmware "+fw.Available+" is in your library", "Running "+fw.Running+". Update from the Firmware page.")
	}
	if snap.CollectedAt.Before(time.Now().Add(-time.Duration(set.StaleHours) * time.Hour)) {
		add(Info, "Login", fmt.Sprintf("Data is more than %d hours old", set.StaleHours), "Last read "+snap.CollectedAt.Format("Jan 2 15:04"))
	}
	return out
}

// fleetIssues looks across phones: the same extension on two phones, for example.
func fleetIssues(snaps map[string]*grandstream.Snapshot, names map[string]string) []Issue {
	type use struct{ phone, server string }
	seen := map[string][]use{}
	for id, s := range snaps {
		if s == nil {
			continue
		}
		for _, a := range s.Accounts {
			if !a.Active || a.UserID == "" || a.Server == "" {
				continue
			}
			key := strings.ToLower(a.UserID + "@" + a.Server)
			seen[key] = append(seen[key], use{names[id], a.Server})
		}
	}
	var out []Issue
	for key, uses := range seen {
		if len(uses) < 2 {
			continue
		}
		var phones []string
		for _, u := range uses {
			phones = append(phones, u.phone)
		}
		sort.Strings(phones)
		ext := key[:strings.Index(key, "@")]
		out = append(out, Issue{Severity: Warning, Category: "Accounts", PhoneName: "Fleet",
			Title:  fmt.Sprintf("Extension %s is on %d phones", ext, len(uses)),
			Detail: strings.Join(phones, ", ") + " all register " + ext + " with " + uses[0].server + ". Unless the PBX allows multiple registrations, they'll knock each other off.",
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

func sortIssues(is []Issue) {
	sort.SliceStable(is, func(i, j int) bool {
		if is[i].Severity.rank() != is[j].Severity.rank() {
			return is[i].Severity.rank() < is[j].Severity.rank()
		}
		return is[i].PhoneName < is[j].PhoneName
	})
}

func displayName(p store.Phone, snap *grandstream.Snapshot) string {
	if p.Name != "" {
		return p.Name
	}
	if snap != nil {
		for _, a := range snap.Accounts {
			if a.Active && a.UserID != "" {
				if a.Display != "" {
					return a.Display + " (" + a.UserID + ")"
				}
				return a.UserID
			}
		}
	}
	return p.Host
}
