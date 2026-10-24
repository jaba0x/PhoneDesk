package core

import "github.com/jaba0x/phonedesk/internal/store"

// Check is one of the rules the dashboard applies. Every check can be
// switched off in Settings; the ones off are listed in Settings.ChecksOff.
type Check struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Detail   string   `json:"detail"`
	Severity Severity `json:"severity"`
	Group    string   `json:"group"`
	// DefaultOff marks checks that start switched off on a fresh install.
	DefaultOff bool `json:"defaultOff"`
}

// Checks is the catalogue, in the order Settings shows it.
var Checks = []Check{
	{ID: "offline", Label: "Phone is offline", Detail: "The web port doesn't answer.", Severity: Critical, Group: "Reachability"},
	{ID: "login-failed", Label: "Web login failed", Detail: "The phone refused the credentials.", Severity: Critical, Group: "Reachability"},
	{ID: "read-failed", Label: "Reading the phone failed", Detail: "The phone is up but the read didn't finish.", Severity: Warning, Group: "Reachability"},
	{ID: "unregistered", Label: "Account not registered", Detail: "An active account whose line reports failed registration.", Severity: Critical, Group: "Accounts"},
	{ID: "no-server", Label: "Account has a user ID but no SIP server", Detail: "Active, half-configured accounts. Empty active accounts (factory state) are never flagged.", Severity: Warning, Group: "Accounts"},
	{ID: "no-account", Label: "No usable SIP account", Detail: "No active account with a user ID or server; the phone can't make or take calls.", Severity: Warning, Group: "Accounts"},
	{ID: "duplicate-ext", Label: "Same extension on two phones", Detail: "The same user ID registered with the same server from more than one phone.", Severity: Warning, Group: "Accounts"},
	{ID: "default-password", Label: "Factory admin password still accepted", Detail: "The phone logs in with admin / admin.", Severity: Critical, Group: "Security"},
	{ID: "plain-http", Label: "Web access over plain HTTP", Detail: "PhoneDesk reached the phone over HTTP, so the admin password travels in clear text.", Severity: Warning, Group: "Security"},
	{ID: "telnet", Label: "Telnet open", Detail: "TCP 23 answers.", Severity: Critical, Group: "Security"},
	{ID: "ssh", Label: "SSH open", Detail: "TCP 22 answers. Fine if you use it.", Severity: Info, Group: "Security"},
	{ID: "no-ntp", Label: "No NTP server", Detail: "Timestamps drift and TLS registrations can fail.", Severity: Warning, Group: "Settings"},
	{ID: "no-syslog", Label: "No syslog server", Detail: "Events only live in the phone's own buffer.", Severity: Info, Group: "Settings"},
	{ID: "option66", Label: "DHCP option 66/43 can override the servers", Detail: "Any DHCP server on the phone's network can point it at another provisioning server.", Severity: Info, Group: "Settings", DefaultOff: true},
	{ID: "firmware", Label: "Newer firmware in the library", Detail: "An image for the phone's family that's newer than what it runs.", Severity: Info, Group: "Firmware"},
	{ID: "stale", Label: "Data older than the limit", Detail: "The phone hasn't been read for longer than the hours set in Monitoring.", Severity: Info, Group: "Freshness"},
}

// DefaultChecksOff lists the checks a fresh install starts without.
func DefaultChecksOff() []string {
	var out []string
	for _, c := range Checks {
		if c.DefaultOff {
			out = append(out, c.ID)
		}
	}
	return out
}

// KnownCheck reports whether id names a check in the catalogue.
func KnownCheck(id string) bool {
	for _, c := range Checks {
		if c.ID == id {
			return true
		}
	}
	return false
}

// checkOn reports whether a check is switched on in the settings.
func checkOn(set store.Settings, id string) bool {
	for _, off := range set.ChecksOff {
		if off == id {
			return false
		}
	}
	return true
}
