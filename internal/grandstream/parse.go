package grandstream

import (
	"regexp"
	"strings"
)

// Build turns the values read from a phone into a Snapshot. It's tolerant:
// firmware differs in which status keys it answers, so each fact is taken
// from the first key that has something in it.
func Build(values map[string]string, lines []Line, linesKnown bool) Snapshot {
	s := Snapshot{Values: values, Lines: lines, LinesKnown: linesKnown}
	first := func(keys ...string) string {
		for _, k := range keys {
			if v := strings.TrimSpace(values[k]); v != "" {
				return v
			}
		}
		return ""
	}
	f := &s.Facts
	f.Vendor = first("vendor_fullname", "vendor_name")
	f.Model = NormalizeModel(first("Pphone_model", "phone_model", "product_model"))
	f.Family = Family(f.Model)
	f.Firmware = first("prog", "prog_version", "P68")
	f.Boot = first("boot", "P69")
	f.Core = first("core", "P70")
	f.Base = first("base", "P45")
	f.DSP = first("dsp")
	f.MAC = NormalizeMAC(first("mac", "P67"))
	f.IP = first("ip", "ipaddr", "Pipv4")
	f.Hardware = first("hwversion")
	f.PN = first("pn", "P1397")
	f.Uptime = first("uptime", "sysuptime", "system_uptime")

	for i, a := range Accounts {
		acc := Account{Index: i + 1, Registered: -1}
		acc.Active = values[a.Active] == "1"
		acc.Name = values[a.Name]
		acc.Display = values[a.Display]
		acc.UserID = values[a.UserID]
		acc.AuthID = values[a.AuthID]
		acc.Server = values[a.Server]
		acc.Proxy = values[a.Proxy]
		// an account with nothing in it isn't configured; keep the first one anyway
		if i > 0 && !acc.Active && acc.UserID == "" && acc.Server == "" && acc.Name == "" {
			continue
		}
		s.Accounts = append(s.Accounts, acc)
	}
	// line status tells us which accounts are registered: on Grandstream
	// phones a line is "failed" when its account can't register and idle
	// otherwise, so an account with any non-failed line is up
	if linesKnown && len(lines) > 0 {
		s.RegistrationKnown = true
		byAcct := map[int]string{}
		for _, l := range lines {
			cur := byAcct[l.Account]
			if cur == "" || (cur == "failed" && l.State != "failed") {
				byAcct[l.Account] = l.State
			}
		}
		for i := range s.Accounts {
			a := &s.Accounts[i]
			if !a.Active {
				continue
			}
			st, ok := byAcct[a.Index-1]
			switch {
			case !ok:
				a.Registered = -1
			case st == "failed":
				a.Registered = 0
			default:
				a.Registered = 1
			}
		}
	}

	s.Network.AddressType = Display(PDHCP, values[PDHCP])
	if s.Network.AddressType == "" && values["PIPStatus"] != "" {
		s.Network.AddressType = Display(PDHCP, values["PIPStatus"]) // GXP status page key
	}
	if s.Network.AddressType == "" {
		s.Network.AddressType = ""
	}
	s.Network.VLAN = values[PVLANTag]
	s.Network.LLDP = boolOf(values, PLLDP)

	s.Time.NTPServer = values[PNTPServer]
	s.Time.Zone = values[PTimeZone]
	s.Time.SelfZone = values[PSelfTimeZone]

	s.Upgrade.FirmwareVia = Display(PFirmwareVia, values[PFirmwareVia])
	s.Upgrade.FirmwarePath = values[PFirmwarePath]
	s.Upgrade.ConfigVia = Display(PConfigVia, values[PConfigVia])
	s.Upgrade.ConfigPath = values[PConfigPath]
	s.Upgrade.Auto = boolOf(values, PAutoUpgrade)
	s.Upgrade.CheckInterval = values[PCheckInterval]
	s.Upgrade.CheckMode = Display(PAlwaysCheck, values[PAlwaysCheck])
	s.Upgrade.Option66 = boolOf(values, POption66)

	s.Logging.Server = values[PSyslogServer]
	s.Logging.Level = Display(PSyslogLevel, values[PSyslogLevel])
	return s
}

func boolOf(values map[string]string, key string) *bool {
	v, ok := values[key]
	if !ok || v == "" {
		return nil
	}
	b := v == "1"
	return &b
}

var macClean = regexp.MustCompile(`[^0-9a-f]`)

// NormalizeMAC writes a MAC as c0:74:ad:12:34:56, or returns "" if it isn't one.
func NormalizeMAC(s string) string {
	h := macClean.ReplaceAllString(strings.ToLower(s), "")
	if len(h) != 12 {
		return ""
	}
	var parts []string
	for i := 0; i < 12; i += 2 {
		parts = append(parts, h[i:i+2])
	}
	return strings.Join(parts, ":")
}

// NormalizeModel upper-cases model names and drops vendor noise.
func NormalizeModel(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "Grandstream "), "GRANDSTREAM ")
	return strings.ToUpper(s)
}

// families maps model prefixes to the firmware file family: a phone model
// GRP2613 downloads grp2610fw.bin, a GXP2170 downloads gxp2130fw.bin, and so
// on. Longer prefixes win. Models not listed fall back to their own name.
var families = []struct {
	prefix string
	family string
}{
	{"GXP1610", "gxp1600"}, {"GXP1615", "gxp1600"}, {"GXP1620", "gxp1600"}, {"GXP1625", "gxp1600"}, {"GXP1628", "gxp1600"}, {"GXP1630", "gxp1600"},
	{"GXP1760", "gxp1760"}, {"GXP1780", "gxp1760"}, {"GXP1782", "gxp1760"},
	{"GXP2130", "gxp2130"}, {"GXP2135", "gxp2130"}, {"GXP2140", "gxp2130"}, {"GXP2160", "gxp2130"}, {"GXP2170", "gxp2130"},
	{"GXP1400", "gxp1400"}, {"GXP1405", "gxp1400"}, {"GXP1450", "gxp1450"},
	{"GXP2100", "gxp2100"}, {"GXP2110", "gxp2100"}, {"GXP2120", "gxp2100"}, {"GXP2124", "gxp2124"},
	{"GRP2601", "grp2600"}, {"GRP2602", "grp2600"}, {"GRP2603", "grp2600"}, {"GRP2604", "grp2600"},
	{"GRP2612", "grp2610"}, {"GRP2613", "grp2610"}, {"GRP2614", "grp2610"}, {"GRP2615", "grp2610"}, {"GRP2616", "grp2610"},
	{"GRP2624", "grp2624"}, {"GRP2634", "grp2634"}, {"GRP2636", "grp2636"},
	{"GRP2650", "grp2650"}, {"GRP2670", "grp2670"},
	{"GXV3350", "gxv3350"}, {"GXV3370", "gxv3370"}, {"GXV3380", "gxv3380"}, {"GXV3450", "gxv3450"}, {"GXV3470", "gxv3470"}, {"GXV3480", "gxv3480"},
	{"WP810", "wp810"}, {"WP820", "wp820"}, {"WP822", "wp822"}, {"WP825", "wp825"},
	{"DP750", "dp750"}, {"DP752", "dp752"}, {"DP760", "dp760"},
	{"HT801", "ht80x"}, {"HT802", "ht80x"}, {"HT812", "ht81x"}, {"HT814", "ht81x"}, {"HT818", "ht81x"},
}

// Family returns the firmware family for a model. The family is the file name
// the phone asks its firmware server for, minus "fw.bin".
func Family(model string) string {
	m := NormalizeModel(model)
	if m == "" {
		return ""
	}
	best := ""
	bestLen := 0
	for _, f := range families {
		if strings.HasPrefix(m, f.prefix) && len(f.prefix) > bestLen {
			best, bestLen = f.family, len(f.prefix)
		}
	}
	if best != "" {
		return best
	}
	// unknown model: use its own name, letters and digits only
	return strings.ToLower(regexp.MustCompile(`[^A-Za-z0-9]`).ReplaceAllString(m, ""))
}

// FirmwareFile is the file name a phone of that family downloads.
func FirmwareFile(family string) string {
	if family == "" {
		return ""
	}
	return family + "fw.bin"
}

// ShortState is the one-word label used in lists.
func (a Account) ShortState() string {
	switch {
	case !a.Active:
		return "off"
	case a.Registered == 1:
		return "registered"
	case a.Registered == 0:
		return "failed"
	}
	return "unknown"
}
