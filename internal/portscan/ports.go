package portscan

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// CommonPorts is used when -tcp/-udp is given with no explicit port
// argument: a curated set of the ports most likely to be running a service
// worth enumerating, without the cost of a full 1-65535 sweep.
var CommonPorts = []int{
	21, 22, 23, 25, 53, 67, 68, 69, 80, 88, 110, 111, 123, 135, 137, 138, 139,
	143, 161, 162, 389, 443, 445, 465, 500, 512, 513, 514, 515, 587, 631, 636,
	873, 993, 995, 1025, 1080, 1194, 1433, 1434, 1521, 1723, 2049, 2181, 2375,
	3000, 3128, 3268, 3306, 3389, 3690, 4444, 4789, 5000, 5060, 5222, 5353,
	5432, 5601, 5672, 5900, 5985, 5986, 6379, 6443, 6667, 7001, 8000, 8008,
	8080, 8081, 8088, 8443, 8888, 9000, 9042, 9090, 9092, 9200, 9300, 9418,
	11211, 15672, 20000, 27017, 27018, 28017, 32768, 50000,
}

// MaxPort is the largest valid TCP/UDP port number.
const MaxPort = 65535

// ParsePorts parses a port specification into a sorted, deduplicated list of
// port numbers. An empty spec returns CommonPorts. Otherwise spec is a
// comma-separated list of single ports ("22"), ranges ("1-1000"), or a mix
// ("22,80,1000-2000"). "-" alone (or "1-65535") means every port.
func ParsePorts(spec string) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return append([]int(nil), CommonPorts...), nil
	}
	if spec == "-" || spec == "*" {
		return AllPorts(), nil
	}

	seen := make(map[int]struct{})
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if lo, hi, ok := strings.Cut(part, "-"); ok {
			loN, err := parsePort(lo)
			if err != nil {
				return nil, err
			}
			hiN, err := parsePort(hi)
			if err != nil {
				return nil, err
			}
			if loN > hiN {
				return nil, fmt.Errorf("portscan: invalid range %q (start > end)", part)
			}
			for p := loN; p <= hiN; p++ {
				seen[p] = struct{}{}
			}
			continue
		}
		p, err := parsePort(part)
		if err != nil {
			return nil, err
		}
		seen[p] = struct{}{}
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("portscan: no ports in spec %q", spec)
	}

	out := make([]int, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Ints(out)
	return out, nil
}

// AllPorts returns every port from 1 to MaxPort, for -p.
func AllPorts() []int {
	out := make([]int, MaxPort)
	for i := range out {
		out[i] = i + 1
	}
	return out
}

func parsePort(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("portscan: %q is not a valid port number", s)
	}
	if n < 1 || n > MaxPort {
		return 0, fmt.Errorf("portscan: port %d out of range (1-%d)", n, MaxPort)
	}
	return n, nil
}
