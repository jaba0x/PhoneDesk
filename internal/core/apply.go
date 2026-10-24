package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jaba0x/phonedesk/internal/grandstream"
	"github.com/jaba0x/phonedesk/internal/store"
)

// Preview lists what writing values to a phone would change, against its
// last reading. Phones that were never read show every value as a change.
type Preview struct {
	PhoneID   string               `json:"phoneId"`
	PhoneName string               `json:"phoneName"`
	Changes   []grandstream.Change `json:"changes"`
	Changed   int                  `json:"changed"`
	Note      string               `json:"note"`
}

func (m *Manager) Preview(ids []string, values map[string]string) []Preview {
	var out []Preview
	for _, id := range ids {
		p, ok := m.st.Phone(id)
		if !ok {
			continue
		}
		m.mu.Lock()
		snap := m.snaps[id]
		m.mu.Unlock()
		cur := map[string]string{}
		pv := Preview{PhoneID: id, PhoneName: displayName(p, snap)}
		if snap != nil {
			cur = snap.Values
		} else {
			pv.Note = "Never read; every value counts as a change."
		}
		pv.Changes = grandstream.Diff(cur, values)
		for _, c := range pv.Changes {
			if c.Changed {
				pv.Changed++
			}
		}
		out = append(out, pv)
	}
	return out
}

// ApplyResult is the outcome of writing values to one phone.
type ApplyResult struct {
	PhoneID    string   `json:"phoneId"`
	PhoneName  string   `json:"phoneName"`
	OK         bool     `json:"ok"`
	Written    int      `json:"written"`
	Error      string   `json:"error"`
	Transcript []string `json:"transcript"`
}

// Apply writes values to phones, one after another, and re-reads each.
// Values a phone already has are skipped. With reboot, the phone is
// restarted afterwards (network and some provisioning settings need that).
func (m *Manager) Apply(ids []string, values map[string]string, reboot bool) []ApplyResult {
	var out []ApplyResult
	for _, id := range ids {
		out = append(out, m.applyOne(id, values, reboot))
	}
	m.changed()
	return out
}

func (m *Manager) applyOne(id string, values map[string]string, reboot bool) ApplyResult {
	p, ok := m.st.Phone(id)
	if !ok {
		return ApplyResult{PhoneID: id, Error: "phone not found"}
	}
	m.mu.Lock()
	snap := m.snaps[id]
	m.mu.Unlock()
	res := ApplyResult{PhoneID: id, PhoneName: displayName(p, snap)}
	say := func(f string, a ...any) { res.Transcript = append(res.Transcript, fmt.Sprintf(f, a...)) }
	if p.Disabled {
		res.Error = "phone is disabled"
		return res
	}
	cur := map[string]string{}
	if snap != nil {
		cur = snap.Values
	}
	todo := map[string]string{}
	for _, c := range grandstream.Diff(cur, values) {
		if c.Changed {
			todo[c.Key] = values[c.Key]
		}
	}
	if len(todo) == 0 {
		res.OK = true
		say("Nothing to change; the phone already has these values.")
		return res
	}
	l := m.lock(id)
	l.Lock()
	defer l.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, _, err := m.client(p)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if err := m.login(ctx, p, c); err != nil {
		res.Error = err.Error()
		return res
	}
	defer c.Logout(context.Background())
	say("Logged in over %s.", strings.ToUpper(c.UsedScheme()))
	keys := make([]string, 0, len(todo))
	for k := range todo {
		keys = append(keys, k)
	}
	grandstream.SortKeys(keys)
	for _, k := range keys {
		v := todo[k]
		if grandstream.ParamIndex[k].Secret {
			v = "••••••"
		}
		say("%s = %s  (%s)", k, v, grandstream.Label(k))
	}
	if err := c.SetValues(ctx, todo); err != nil {
		res.Error = "The phone refused the change: " + err.Error()
		say("Refused: %s", err.Error())
		return res
	}
	res.Written = len(todo)
	say("Written: %d value(s).", len(todo))
	if _, changed := todo[grandstream.PAdminPassword]; changed {
		say("The admin password changed. Update the credentials in PhoneDesk, or the next read will fail.")
	}
	if reboot {
		if err := c.Reboot(ctx); err != nil {
			say("Reboot refused: %s. Restart the phone yourself for network changes to apply.", err.Error())
		} else {
			say("Rebooting.")
			st := m.stat(id)
			m.mu.Lock()
			st.LastCollect = time.Time{} // read again when it's back
			m.mu.Unlock()
		}
	} else {
		// read back what the phone has now
		c.Logout(context.Background())
		if snap2, err := m.read(ctx, p); err == nil {
			m.mu.Lock()
			m.snaps[id] = &snap2
			st := m.status[id]
			if st != nil {
				st.LastCollect, st.CollectError, st.AuthFailed = snap2.CollectedAt, "", false
			}
			m.mu.Unlock()
			m.st.SaveSnapshot(id, snap2)
			m.st.SaveBackup(id, snap2.Values, m.st.Settings().KeepBackups)
			say("Read back the phone's settings.")
		} else {
			say("Couldn't read the phone back: %s", err.Error())
		}
	}
	res.OK = true
	return res
}

// ProvisioningFiles renders cfg<mac>.xml for each phone with the given
// values. Phones without a known MAC are listed in missing.
func (m *Manager) ProvisioningFiles(ids []string, values map[string]string) (files map[string][]byte, missing []string) {
	files = map[string][]byte{}
	for _, id := range ids {
		p, ok := m.st.Phone(id)
		if !ok {
			continue
		}
		m.mu.Lock()
		snap := m.snaps[id]
		m.mu.Unlock()
		mac := p.MAC
		if mac == "" && snap != nil {
			mac = snap.Facts.MAC
		}
		if mac == "" {
			missing = append(missing, displayName(p, snap))
			continue
		}
		files[grandstream.ConfigFileName(mac)] = grandstream.ProvisioningXML(values, mac)
	}
	return files, missing
}

// PhoneValues returns the P-values from a phone's last reading as template
// lines, to start a template from a phone that's set up the way you like.
func (m *Manager) PhoneValues(id string) (map[string]string, error) {
	m.mu.Lock()
	snap := m.snaps[id]
	m.mu.Unlock()
	if snap == nil {
		return nil, errors.New("the phone hasn't been read yet")
	}
	out := map[string]string{}
	for k, v := range snap.Values {
		if grandstream.IsPKey(k) {
			out[k] = v
		}
	}
	return out, nil
}

var _ = store.Phone{}
