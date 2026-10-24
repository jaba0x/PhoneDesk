package grandstream

import "time"

// Snapshot is everything PhoneDesk read from a phone in one go.
type Snapshot struct {
	CollectedAt time.Time `json:"collectedAt"`
	Duration    string    `json:"duration"`
	Scheme      string    `json:"scheme"` // "https" or "http": how the phone was reached

	Facts             Facts             `json:"facts"`
	Accounts          []Account         `json:"accounts"`
	Lines             []Line            `json:"lines"`
	Network           Network           `json:"network"`
	Time              TimeSettings      `json:"time"`
	Upgrade           Upgrade           `json:"upgrade"`
	Logging           Logging           `json:"logging"`
	Security          Security          `json:"security"`
	Values            map[string]string `json:"values"` // every value read, by key
	Raw               map[string]string `json:"raw"`    // request -> response body, as it came
	Errors            []string          `json:"errors"` // things that didn't work but didn't stop the read
	LinesKnown        bool              `json:"linesKnown"`
	RegistrationKnown bool              `json:"registrationKnown"`
}

type Facts struct {
	Vendor   string `json:"vendor"`
	Model    string `json:"model"`    // "GRP2613"
	Family   string `json:"family"`   // firmware family, "grp2610"
	Firmware string `json:"firmware"` // "1.0.13.31"
	Boot     string `json:"boot"`
	Core     string `json:"core"`
	Base     string `json:"base"`
	DSP      string `json:"dsp"`
	MAC      string `json:"mac"` // "c0:74:ad:12:34:56"
	IP       string `json:"ip"`
	Hardware string `json:"hardware"`
	PN       string `json:"pn"`
	Uptime   string `json:"uptime"`
}

// Account is one SIP account as configured on the phone.
type Account struct {
	Index      int    `json:"index"` // 1-based
	Active     bool   `json:"active"`
	Name       string `json:"name"`
	Display    string `json:"display"`
	UserID     string `json:"userId"`
	AuthID     string `json:"authId"`
	Server     string `json:"server"`
	Proxy      string `json:"proxy"`
	Registered int    `json:"registered"` // 1 registered, 0 not, -1 unknown
}

// Line is the live state of a line key from api-get_line_status.
type Line struct {
	Line         int    `json:"line"`
	State        string `json:"state"` // idle, ringing, calling, incall, hold, failed, unknown
	Account      int    `json:"account"`
	Active       bool   `json:"active"`
	RemoteName   string `json:"remoteName"`
	RemoteNumber string `json:"remoteNumber"`
}

type Network struct {
	AddressType string `json:"addressType"` // "DHCP", "Static", "PPPoE"
	VLAN        string `json:"vlan"`
	LLDP        *bool  `json:"lldp"`
}

type TimeSettings struct {
	NTPServer string `json:"ntpServer"`
	Zone      string `json:"zone"`
	SelfZone  string `json:"selfZone"`
}

type Upgrade struct {
	FirmwareVia   string `json:"firmwareVia"` // TFTP, HTTP, HTTPS...
	FirmwarePath  string `json:"firmwarePath"`
	ConfigVia     string `json:"configVia"`
	ConfigPath    string `json:"configPath"`
	Auto          *bool  `json:"auto"`
	CheckInterval string `json:"checkInterval"`
	CheckMode     string `json:"checkMode"` // "Always", ...
	Option66      *bool  `json:"option66"`
}

type Logging struct {
	Server string `json:"server"`
	Level  string `json:"level"`
}

// Security is what PhoneDesk could see about the management plane. Values
// are "open", "closed" or "filtered" for probed ports, as in SwitchDesk.
type Security struct {
	HTTPPort     string `json:"httpPort"`  // TCP 80
	HTTPSPort    string `json:"httpsPort"` // TCP 443
	SSHPort      string `json:"sshPort"`   // TCP 22
	TelnetPort   string `json:"telnetPort"`
	DefaultAdmin bool   `json:"defaultAdmin"` // logged in with the factory password
	PlainWeb     bool   `json:"plainWeb"`     // reached over http, not https
}
