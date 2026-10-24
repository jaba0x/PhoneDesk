package grandstream

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Template is a named set of P-values to push to phones, or to hand to a
// provisioning server as cfg<mac>.xml.
type Template struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Notes     string            `json:"notes"`
	Values    map[string]string `json:"values"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

// Change is one value that would be written.
type Change struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	From    string `json:"from"`
	To      string `json:"to"`
	Secret  bool   `json:"secret"`
	Changed bool   `json:"changed"` // false when the phone already has this value
}

// Diff lists what applying values to a phone with the given snapshot changes.
// Secret values are always "changed" since phones never report them back.
func Diff(current map[string]string, values map[string]string) []Change {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	SortKeys(keys)
	var out []Change
	for _, k := range keys {
		p := ParamIndex[k]
		from, has := current[k]
		ch := Change{Key: k, Label: Label(k), From: from, To: values[k], Secret: p.Secret}
		ch.Changed = p.Secret || !has || from != values[k]
		if p.Secret {
			ch.From, ch.To = "", "••••••"
		}
		out = append(out, ch)
	}
	return out
}

// ParseLines reads "P47 = 10.0.0.5" lines (also "P47=…", "P47: …"). Blank
// lines and # comments are skipped. Values may be empty, which clears the
// setting on the phone.
func ParseLines(text string) (map[string]string, error) {
	out := map[string]string{}
	var bad []string
	for n, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		sep := strings.IndexAny(line, "=:")
		if sep < 0 {
			bad = append(bad, fmt.Sprintf("line %d: %q has no value", n+1, line))
			continue
		}
		k := strings.ToUpper(strings.TrimSpace(line[:sep]))
		v := strings.TrimSpace(line[sep+1:])
		if !IsPKey(k) {
			bad = append(bad, fmt.Sprintf("line %d: %q is not a P-value", n+1, k))
			continue
		}
		out[k] = v
	}
	if len(bad) > 0 {
		return out, errors.New(strings.Join(bad, "\n"))
	}
	return out, nil
}

// FormatLines writes values as "P47 = value" lines with a comment naming the
// setting, in numeric order.
func FormatLines(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	SortKeys(keys)
	var b strings.Builder
	for _, k := range keys {
		if l := Label(k); l != k {
			fmt.Fprintf(&b, "# %s\n", l)
		}
		fmt.Fprintf(&b, "%s = %s\n", k, values[k])
	}
	return b.String()
}

// ProvisioningXML writes values in Grandstream's XML config format. If mac is
// given it's noted in a comment; the file itself is the same for every phone.
func ProvisioningXML(values map[string]string, mac string) []byte {
	keys := make([]string, 0, len(values))
	for k := range values {
		if IsPKey(k) {
			keys = append(keys, k)
		}
	}
	SortKeys(keys)
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString("<!-- Made with PhoneDesk")
	if mac != "" {
		b.WriteString(" for " + mac)
	}
	b.WriteString(" on " + time.Now().Format("2006-01-02 15:04") + " -->\n")
	b.WriteString(`<gs_provision version="1">` + "\n" + `  <config version="1">` + "\n")
	for _, k := range keys {
		var v bytes.Buffer
		xml.EscapeText(&v, []byte(values[k])) //nolint:errcheck
		if l := Label(k); l != k {
			fmt.Fprintf(&b, "    <!-- %s -->\n", strings.ReplaceAll(l, "--", "-"))
		}
		fmt.Fprintf(&b, "    <%s>%s</%s>\n", k, v.String(), k)
	}
	b.WriteString("  </config>\n</gs_provision>\n")
	return b.Bytes()
}

// ConfigFileName is what a phone asks its provisioning server for.
func ConfigFileName(mac string) string {
	m := strings.ReplaceAll(NormalizeMAC(mac), ":", "")
	if m == "" {
		return "cfg.xml"
	}
	return "cfg" + m + ".xml"
}

// GroupedParams returns known parameters grouped in display order, for
// building an editor.
func GroupedParams() []struct {
	Group  string  `json:"group"`
	Params []Param `json:"params"`
} {
	type grp = struct {
		Group  string  `json:"group"`
		Params []Param `json:"params"`
	}
	var out []grp
	idx := map[string]int{}
	for _, p := range Params {
		i, ok := idx[p.Group]
		if !ok {
			i = len(out)
			idx[p.Group] = i
			out = append(out, grp{Group: p.Group})
		}
		out[i].Params = append(out[i].Params, p)
	}
	return out
}

// Keys returns a template's keys in order (helper for logs).
func (t Template) Keys() []string {
	keys := make([]string, 0, len(t.Values))
	for k := range t.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	SortKeys(keys)
	return keys
}
