// Package grandstream talks to Grandstream IP phones (GXP, GRP and friends)
// over their web API, and turns what comes back into something readable.
//
// Everything on a Grandstream phone is a "P-value": P47 is the SIP server of
// account 1, P30 the NTP server, and so on. The same numbers are used in
// provisioning files, so a template built here is also a provisioning file.
package grandstream

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Kind says how a value is shown and edited.
type Kind string

const (
	Text     Kind = "text"
	Password Kind = "password"
	Bool     Kind = "bool" // "0" / "1"
	Enum     Kind = "enum"
	Number   Kind = "number"
)

// Param describes one known P-value.
type Param struct {
	Key     string            `json:"key"`   // "P47"
	Label   string            `json:"label"` // "SIP server"
	Group   string            `json:"group"` // "Account 1", "Network", "Time", ...
	Kind    Kind              `json:"kind"`
	Options map[string]string `json:"options,omitempty"` // enum value -> label
	Help    string            `json:"help,omitempty"`
	// Secret values are never read back from the phone (the phone doesn't
	// return them) and never stored in snapshots.
	Secret bool `json:"secret,omitempty"`
}

// account holds the P-value numbers of one SIP account. Grandstream numbers
// accounts 1-6 inconsistently, so they're spelled out.
type account struct {
	Active, Name, Server, Proxy, UserID, AuthID, AuthPass, Display string
}

// Accounts lists the P-values of accounts 1 to 6 in order.
var Accounts = []account{
	{"P271", "P270", "P47", "P48", "P35", "P36", "P34", "P3"},
	{"P401", "P417", "P402", "P403", "P404", "P405", "P406", "P407"},
	{"P501", "P517", "P502", "P503", "P504", "P505", "P506", "P507"},
	{"P601", "P617", "P602", "P603", "P604", "P605", "P606", "P607"},
	{"P1701", "P1717", "P1702", "P1703", "P1704", "P1705", "P1706", "P1707"},
	{"P1801", "P1817", "P1802", "P1803", "P1804", "P1805", "P1806", "P1807"},
}

// Well-known values outside the accounts. Numbers come from Grandstream's
// published P-value lists and provisioning templates.
const (
	PAdminPassword = "P2"
	PUserPassword  = "P196"
	PNTPServer     = "P30"
	PTimeZone      = "P64"
	PSelfTimeZone  = "P6767"
	PDHCP          = "P8" // 0 = DHCP, 1 = static
	PStaticIP1     = "P9"
	PVLANTag       = "P51"
	PFirmwareVia   = "P212" // 0 TFTP, 1 HTTP, 2 HTTPS
	PFirmwarePath  = "P192"
	PConfigVia     = "P240"
	PConfigPath    = "P237"
	PAutoUpgrade   = "P194" // 0 no, 1 yes
	PCheckInterval = "P193" // minutes
	PAlwaysCheck   = "P238" // 0 always, 1 when prefix/suffix changes, 2 skip
	PFirmwarePre   = "P232"
	PFirmwareSuf   = "P233"
	PConfigPre     = "P234"
	PConfigSuf     = "P235"
	POption66      = "P145" // allow DHCP option 66/43 to override server
	PSyslogServer  = "P207"
	PSyslogLevel   = "P208"
	PPnP3CX        = "P1414"
	PLLDP          = "P1684"
	PDialPlan      = "P290"
)

// Params is every value PhoneDesk knows by name, in display order.
var Params []Param

// ParamIndex finds a Param by key.
var ParamIndex = map[string]Param{}

func init() {
	yesNo := map[string]string{"0": "No", "1": "Yes"}
	add := func(p Param) {
		Params = append(Params, p)
		ParamIndex[p.Key] = p
	}
	for i, a := range Accounts {
		g := fmt.Sprintf("Account %d", i+1)
		add(Param{Key: a.Active, Label: "Active", Group: g, Kind: Bool, Options: yesNo})
		add(Param{Key: a.Name, Label: "Account name", Group: g, Kind: Text, Help: "Shown on the phone's line key"})
		add(Param{Key: a.Server, Label: "SIP server", Group: g, Kind: Text})
		add(Param{Key: a.Proxy, Label: "Outbound proxy", Group: g, Kind: Text})
		add(Param{Key: a.UserID, Label: "SIP user ID", Group: g, Kind: Text, Help: "Usually the extension"})
		add(Param{Key: a.AuthID, Label: "Authenticate ID", Group: g, Kind: Text})
		add(Param{Key: a.AuthPass, Label: "Authenticate password", Group: g, Kind: Password, Secret: true})
		add(Param{Key: a.Display, Label: "Display name", Group: g, Kind: Text})
	}
	add(Param{Key: PAdminPassword, Label: "Admin password", Group: "Web access", Kind: Password, Secret: true, Help: "Changing it also changes the password PhoneDesk needs; update the credentials afterwards"})
	add(Param{Key: PUserPassword, Label: "User password", Group: "Web access", Kind: Password, Secret: true})

	add(Param{Key: PDHCP, Label: "Address type", Group: "Network", Kind: Enum, Options: map[string]string{"0": "DHCP", "1": "Static", "2": "PPPoE"}})
	add(Param{Key: PVLANTag, Label: "802.1Q VLAN tag", Group: "Network", Kind: Number, Help: "0 = untagged"})
	add(Param{Key: PLLDP, Label: "LLDP", Group: "Network", Kind: Bool, Options: yesNo})

	add(Param{Key: PNTPServer, Label: "NTP server", Group: "Time", Kind: Text})
	add(Param{Key: PTimeZone, Label: "Time zone", Group: "Time", Kind: Text, Help: "Grandstream zone code, e.g. TZR-4 or EST5EDT"})
	add(Param{Key: PSelfTimeZone, Label: "Self-defined time zone", Group: "Time", Kind: Text, Help: "POSIX rule, e.g. MTZ+6MDT+5,M3.2.0,M11.1.0"})

	add(Param{Key: PFirmwareVia, Label: "Upgrade via", Group: "Firmware upgrade", Kind: Enum, Options: map[string]string{"0": "TFTP", "1": "HTTP", "2": "HTTPS", "3": "FTP", "4": "FTPS"}})
	add(Param{Key: PFirmwarePath, Label: "Firmware server path", Group: "Firmware upgrade", Kind: Text, Help: "Host, or host/folder, without http://"})
	add(Param{Key: PFirmwarePre, Label: "Firmware file prefix", Group: "Firmware upgrade", Kind: Text})
	add(Param{Key: PFirmwareSuf, Label: "Firmware file postfix", Group: "Firmware upgrade", Kind: Text})
	add(Param{Key: PAutoUpgrade, Label: "Automatic upgrade", Group: "Firmware upgrade", Kind: Bool, Options: yesNo})
	add(Param{Key: PCheckInterval, Label: "Check every (minutes)", Group: "Firmware upgrade", Kind: Number})
	add(Param{Key: PAlwaysCheck, Label: "Check for new firmware", Group: "Firmware upgrade", Kind: Enum, Options: map[string]string{"0": "Always", "1": "Only when the prefix or postfix changes", "2": "Never"}})

	add(Param{Key: PConfigVia, Label: "Config upgrade via", Group: "Provisioning", Kind: Enum, Options: map[string]string{"0": "TFTP", "1": "HTTP", "2": "HTTPS", "3": "FTP", "4": "FTPS"}})
	add(Param{Key: PConfigPath, Label: "Config server path", Group: "Provisioning", Kind: Text})
	add(Param{Key: PConfigPre, Label: "Config file prefix", Group: "Provisioning", Kind: Text})
	add(Param{Key: PConfigSuf, Label: "Config file postfix", Group: "Provisioning", Kind: Text})
	add(Param{Key: POption66, Label: "Allow DHCP option 43/66 to override server", Group: "Provisioning", Kind: Bool, Options: yesNo})
	add(Param{Key: PPnP3CX, Label: "3CX auto provision (PnP)", Group: "Provisioning", Kind: Bool, Options: yesNo})

	add(Param{Key: PSyslogServer, Label: "Syslog server", Group: "Logging", Kind: Text})
	add(Param{Key: PSyslogLevel, Label: "Syslog level", Group: "Logging", Kind: Enum, Options: map[string]string{"0": "None", "1": "Debug", "2": "Info", "3": "Warning", "4": "Error"}})

	add(Param{Key: PDialPlan, Label: "Dial plan", Group: "Calls", Kind: Text})
}

// StatusKeys are the non-P keys the web UI asks the phone for. Not every
// firmware answers all of them; whatever comes back is kept in the snapshot.
// StatusKeys are read along with the settings. GRP phones answer the named
// keys; GXP16xx/21xx firmware keeps the same facts under P-numbers, which is
// what their own status pages ask for (P68 prog, P67 MAC, Pipv4 address).
var StatusKeys = []string{
	"vendor_name", "vendor_fullname", "phone_model", "Pphone_model", "product_model",
	"prog", "prog_version", "boot", "core", "base", "dsp", "recovery",
	"mac", "ip", "ipaddr", "netmask", "gateway", "dns1", "dns2",
	"uptime", "sysuptime", "system_uptime", "hwversion", "pn", "serial",
	"P68", "P69", "P70", "P45", "P7033", "P67", "Pipv4", "PIPStatus", "Psubnet_web", "Pgateway_web", "Pdns1_web", "Pdns2_web", "P1397",
}

// CollectKeys is everything asked from a phone in one read: the known
// settings (except secrets, which phones don't return anyway) plus status keys.
func CollectKeys() []string {
	keys := make([]string, 0, len(Params)+len(StatusKeys))
	for _, p := range Params {
		if !p.Secret {
			keys = append(keys, p.Key)
		}
	}
	return append(keys, StatusKeys...)
}

// IsPKey reports whether k looks like a P-value name.
func IsPKey(k string) bool {
	if len(k) < 2 || k[0] != 'P' {
		return false
	}
	_, err := strconv.Atoi(k[1:])
	return err == nil
}

// Label returns a readable name for a key, or the key itself.
func Label(k string) string {
	if p, ok := ParamIndex[k]; ok {
		if strings.HasPrefix(p.Group, "Account") {
			return p.Group + " " + strings.ToLower(p.Label)
		}
		return p.Label
	}
	return k
}

// Display turns a raw value into what the UI shows (enum labels, Yes/No).
func Display(k, v string) string {
	p, ok := ParamIndex[k]
	if !ok || p.Options == nil {
		return v
	}
	if l, ok := p.Options[v]; ok {
		return l
	}
	return v
}

// SortKeys orders P-values numerically, other keys alphabetically after them.
func SortKeys(keys []string) {
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		pa, pb := IsPKey(a), IsPKey(b)
		if pa != pb {
			return pa
		}
		if pa {
			x, _ := strconv.Atoi(a[1:])
			y, _ := strconv.Atoi(b[1:])
			return x < y
		}
		return a < b
	})
}
