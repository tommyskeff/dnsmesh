package dns

import (
	"math/rand"
	"net"
	"strings"
	"sync"
)

type entry struct {
	IP     net.IP
	Source string // "pod" or "service"
}

type Table struct {
	mu      sync.RWMutex
	records map[string][]entry
}

func NewTable() *Table {
	return &Table{
		records: make(map[string][]entry),
	}
}

func (t *Table) AddEntry(name string, ip net.IP, source string) {
	name = normalizeName(name)

	t.mu.Lock()
	defer t.mu.Unlock()

	for _, e := range t.records[name] {
		if e.IP.Equal(ip) && e.Source == source {
			return
		}
	}

	t.records[name] = append(t.records[name], entry{IP: ip, Source: source})
}

func (t *Table) RemoveEntry(name string, ip net.IP, source string) {
	name = normalizeName(name)

	t.mu.Lock()
	defer t.mu.Unlock()

	entries := t.records[name]
	for i, e := range entries {
		if e.IP.Equal(ip) && e.Source == source {
			entries[i] = entries[len(entries)-1]
			t.records[name] = entries[:len(entries)-1]
			break
		}
	}

	if len(t.records[name]) == 0 {
		delete(t.records, name)
	}
}

func (t *Table) Lookup(name string) []net.IP {
	name = normalizeName(name)

	t.mu.RLock()
	defer t.mu.RUnlock()

	entries := t.records[name]
	if len(entries) == 0 {
		return nil
	}

	ips := make([]net.IP, len(entries))
	for i, e := range entries {
		ips[i] = e.IP
	}
	return ips
}

func (t *Table) LookupRandom(name string) net.IP {
	ips := t.Lookup(name)
	if len(ips) == 0 {
		return nil
	}
	if len(ips) == 1 {
		return ips[0]
	}
	return ips[rand.Intn(len(ips))]
}

func (t *Table) LookupIPv4(name string) []net.IP {
	ips := t.Lookup(name)
	var ipv4s []net.IP
	for _, ip := range ips {
		if ip.To4() != nil {
			ipv4s = append(ipv4s, ip)
		}
	}
	return ipv4s
}

func (t *Table) LookupIPv6(name string) []net.IP {
	ips := t.Lookup(name)
	var ipv6s []net.IP
	for _, ip := range ips {
		if ip.To4() == nil && ip.To16() != nil {
			ipv6s = append(ipv6s, ip)
		}
	}
	return ipv6s
}

func (t *Table) LookupRandomIPv4(name string) net.IP {
	ips := t.LookupIPv4(name)
	if len(ips) == 0 {
		return nil
	}
	if len(ips) == 1 {
		return ips[0]
	}
	return ips[rand.Intn(len(ips))]
}

func (t *Table) LookupRandomIPv6(name string) net.IP {
	ips := t.LookupIPv6(name)
	if len(ips) == 0 {
		return nil
	}
	if len(ips) == 1 {
		return ips[0]
	}
	return ips[rand.Intn(len(ips))]
}

func (t *Table) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	count := 0
	for _, entries := range t.records {
		count += len(entries)
	}
	return count
}

func (t *Table) GetAllNames() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	names := make([]string, 0, len(t.records))
	for name := range t.records {
		names = append(names, name)
	}
	return names
}

func (t *Table) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records = make(map[string][]entry)
}

func normalizeName(name string) string {
	name = strings.ToLower(name)
	if !strings.HasSuffix(name, ".") {
		name += "."
	}
	return name
}
