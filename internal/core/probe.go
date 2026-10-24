package core

import (
	"errors"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jaba0x/phonedesk/internal/grandstream"
)

var probeTimeout = 3 * time.Second

// ports probed; tests point these at ports known to be free
var httpPortNum, httpsPortNum, sshPortNum, telnetPortNum = "80", "443", "22", "23"

// probePort reports "open" when something accepts a TCP connection, "closed"
// when the phone refuses it, and "filtered" when nothing answers in time.
func probePort(host, port string) string {
	c, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), probeTimeout)
	if err == nil {
		c.Close()
		return "open"
	}
	if errors.Is(err, syscall.ECONNREFUSED) || strings.Contains(err.Error(), "refused") {
		return "closed"
	}
	return "filtered"
}

// applyPortProbes checks the management ports directly. What the phone
// actually answers beats what its settings seem to say.
func applyPortProbes(s *grandstream.Snapshot, host string) {
	var wg sync.WaitGroup
	res := make([]string, 4)
	for i, port := range []string{httpPortNum, httpsPortNum, sshPortNum, telnetPortNum} {
		wg.Add(1)
		go func(i int, port string) {
			defer wg.Done()
			res[i] = probePort(host, port)
		}(i, port)
	}
	wg.Wait()
	s.Security.HTTPPort, s.Security.HTTPSPort, s.Security.SSHPort, s.Security.TelnetPort = res[0], res[1], res[2], res[3]
}
