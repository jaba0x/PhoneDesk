package core

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jaba0x/phonedesk/internal/grandstream"
)

// Found is a phone the scanner saw.
type Found struct {
	Host   string `json:"host"`
	Scheme string `json:"scheme"`
	Known  bool   `json:"known"` // already in the inventory
	Name   string `json:"name"`
}

// ScanProgress is reported while a scan runs.
type ScanProgress struct {
	Done  int     `json:"done"`
	Total int     `json:"total"`
	Found []Found `json:"found"`
}

// ExpandTargets turns "10.0.1.0/24", "10.0.1.20-10.0.1.60", "10.0.1.20-60"
// and single addresses (one per line, or comma separated) into a host list.
func ExpandTargets(text string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	push := func(h string) {
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	for _, raw := range strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == ',' || r == ';' || r == ' ' }) {
		t := strings.TrimSpace(raw)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.Contains(t, "/") {
			ip, ipn, err := net.ParseCIDR(t)
			if err != nil || ip.To4() == nil {
				return nil, fmt.Errorf("%q isn't a network like 10.0.1.0/24", t)
			}
			ones, _ := ipn.Mask.Size()
			if ones < 16 {
				return nil, fmt.Errorf("%s is too big to scan; use /16 or smaller", t)
			}
			for a := ipn.IP.Mask(ipn.Mask); ipn.Contains(a); a = next(a) {
				if ones < 31 && (a.Equal(ipn.IP.Mask(ipn.Mask)) || a.Equal(last(ipn))) {
					continue // network and broadcast
				}
				push(a.String())
			}
			continue
		}
		if from, to, ok := strings.Cut(t, "-"); ok && net.ParseIP(from) != nil {
			hosts, err := expandRange(from, to)
			if err != nil {
				return nil, err
			}
			for _, h := range hosts {
				push(h)
			}
			continue
		}
		push(t)
	}
	if len(out) > 70000 {
		return nil, fmt.Errorf("%d addresses is too many for one scan", len(out))
	}
	return out, nil
}

func next(ip net.IP) net.IP {
	out := make(net.IP, len(ip))
	copy(out, ip)
	for i := len(out) - 1; i >= 0; i-- {
		out[i]++
		if out[i] != 0 {
			break
		}
	}
	return out
}

func last(n *net.IPNet) net.IP {
	out := make(net.IP, len(n.IP))
	for i := range n.IP {
		out[i] = n.IP[i] | ^n.Mask[i]
	}
	return out
}

func expandRange(from, to string) ([]string, error) {
	ip := net.ParseIP(from).To4()
	if ip == nil {
		return nil, fmt.Errorf("%q isn't an IPv4 address", from)
	}
	end := ip[3]
	if n, err := strconv.Atoi(to); err == nil && n >= 0 && n < 256 {
		end = byte(n)
	} else if t := net.ParseIP(to).To4(); t != nil && t[0] == ip[0] && t[1] == ip[1] && t[2] == ip[2] {
		end = t[3]
	} else {
		return nil, fmt.Errorf("range %s-%s must stay inside one /24", from, to)
	}
	var out []string
	for i := int(ip[3]); i <= int(end); i++ {
		out = append(out, fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], i))
	}
	return out, nil
}

// Scan looks for Grandstream phones at the targets and reports progress.
func (m *Manager) Scan(ctx context.Context, hosts []string, progress func(ScanProgress)) []Found {
	known := map[string]string{}
	for _, p := range m.st.Phones() {
		known[strings.ToLower(p.Host)] = displayName(p, nil)
	}
	var mu sync.Mutex
	var found []Found
	done := 0
	report := func() {
		if progress == nil {
			return
		}
		cp := append([]Found(nil), found...)
		progress(ScanProgress{Done: done, Total: len(hosts), Found: cp})
	}
	sem := make(chan struct{}, 64)
	var wg sync.WaitGroup
	lastReport := time.Now()
	for _, h := range hosts {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(h string) {
			defer wg.Done()
			defer func() { <-sem }()
			scheme, err := grandstream.Fingerprint(ctx, h, 4*time.Second)
			mu.Lock()
			defer mu.Unlock()
			done++
			if err == nil {
				name, isKnown := known[strings.ToLower(h)]
				found = append(found, Found{Host: h, Scheme: scheme, Known: isKnown, Name: name})
			}
			if err == nil || time.Since(lastReport) > 400*time.Millisecond || done == len(hosts) {
				lastReport = time.Now()
				report()
			}
		}(h)
	}
	wg.Wait()
	sort.Slice(found, func(i, j int) bool { return ipLess(found[i].Host, found[j].Host) })
	mu.Lock()
	defer mu.Unlock()
	report()
	return found
}

func ipLess(a, b string) bool {
	ia, ib := net.ParseIP(a).To4(), net.ParseIP(b).To4()
	if ia == nil || ib == nil {
		return a < b
	}
	for i := range ia {
		if ia[i] != ib[i] {
			return ia[i] < ib[i]
		}
	}
	return false
}
