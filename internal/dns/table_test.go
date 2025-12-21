package dns

import (
	"net"
	"sync"
	"testing"
)

func TestTable_AddEntry(t *testing.T) {
	table := NewTable()

	ip := net.ParseIP("10.0.0.1")
	table.AddEntry("test.example.com", ip, "pod")

	ips := table.Lookup("test.example.com")
	if len(ips) != 1 {
		t.Errorf("expected 1 IP, got %d", len(ips))
	}
	if !ips[0].Equal(ip) {
		t.Errorf("expected IP %v, got %v", ip, ips[0])
	}
}

func TestTable_AddDuplicateEntry(t *testing.T) {
	table := NewTable()

	ip := net.ParseIP("10.0.0.1")
	table.AddEntry("test.example.com", ip, "pod")
	table.AddEntry("test.example.com", ip, "pod")

	ips := table.Lookup("test.example.com")
	if len(ips) != 1 {
		t.Errorf("expected 1 IP (no duplicate), got %d", len(ips))
	}
}

func TestTable_MultipleIPs(t *testing.T) {
	table := NewTable()

	table.AddEntry("test.example.com", net.ParseIP("10.0.0.1"), "pod")
	table.AddEntry("test.example.com", net.ParseIP("10.0.0.2"), "pod")
	table.AddEntry("test.example.com", net.ParseIP("10.0.0.3"), "service")

	ips := table.Lookup("test.example.com")
	if len(ips) != 3 {
		t.Errorf("expected 3 IPs, got %d", len(ips))
	}
}

func TestTable_RemoveEntry(t *testing.T) {
	table := NewTable()

	ip1 := net.ParseIP("10.0.0.1")
	ip2 := net.ParseIP("10.0.0.2")

	table.AddEntry("test.example.com", ip1, "pod")
	table.AddEntry("test.example.com", ip2, "pod")

	table.RemoveEntry("test.example.com", ip1, "pod")

	ips := table.Lookup("test.example.com")
	if len(ips) != 1 {
		t.Errorf("expected 1 IP after removal, got %d", len(ips))
	}
	if !ips[0].Equal(ip2) {
		t.Errorf("expected IP %v to remain, got %v", ip2, ips[0])
	}
}

func TestTable_RemoveLastEntry(t *testing.T) {
	table := NewTable()

	ip := net.ParseIP("10.0.0.1")
	table.AddEntry("test.example.com", ip, "pod")
	table.RemoveEntry("test.example.com", ip, "pod")

	ips := table.Lookup("test.example.com")
	if len(ips) != 0 {
		t.Errorf("expected 0 IPs after removal, got %d", len(ips))
	}

	names := table.GetAllNames()
	if len(names) != 0 {
		t.Errorf("expected no names in table, got %d", len(names))
	}
}

func TestTable_LookupNotFound(t *testing.T) {
	table := NewTable()

	ips := table.Lookup("nonexistent.example.com")
	if ips != nil {
		t.Errorf("expected nil for nonexistent name, got %v", ips)
	}
}

func TestTable_LookupRandom(t *testing.T) {
	table := NewTable()

	table.AddEntry("test.example.com", net.ParseIP("10.0.0.1"), "pod")
	table.AddEntry("test.example.com", net.ParseIP("10.0.0.2"), "pod")
	table.AddEntry("test.example.com", net.ParseIP("10.0.0.3"), "pod")

	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		ip := table.LookupRandom("test.example.com")
		if ip == nil {
			t.Fatal("expected non-nil IP from LookupRandom")
		}
		seen[ip.String()] = true
	}

	if len(seen) < 2 {
		t.Logf("warning: only saw %d unique IPs in 100 lookups, random distribution may be poor", len(seen))
	}
}

func TestTable_LookupRandomNotFound(t *testing.T) {
	table := NewTable()

	ip := table.LookupRandom("nonexistent.example.com")
	if ip != nil {
		t.Errorf("expected nil for nonexistent name, got %v", ip)
	}
}

func TestTable_NormalizeName(t *testing.T) {
	table := NewTable()

	ip := net.ParseIP("10.0.0.1")

	table.AddEntry("test.example.com.", ip, "pod")

	ips := table.Lookup("test.example.com")
	if len(ips) != 1 {
		t.Errorf("expected 1 IP, got %d", len(ips))
	}

	ips = table.Lookup("TEST.EXAMPLE.COM.")
	if len(ips) != 1 {
		t.Errorf("expected 1 IP for uppercase lookup, got %d", len(ips))
	}
}

func TestTable_Count(t *testing.T) {
	table := NewTable()

	if table.Count() != 0 {
		t.Errorf("expected count 0 for empty table")
	}

	table.AddEntry("a.example.com", net.ParseIP("10.0.0.1"), "pod")
	table.AddEntry("b.example.com", net.ParseIP("10.0.0.2"), "pod")
	table.AddEntry("a.example.com", net.ParseIP("10.0.0.3"), "service")

	if table.Count() != 3 {
		t.Errorf("expected count 3, got %d", table.Count())
	}
}

func TestTable_Clear(t *testing.T) {
	table := NewTable()

	table.AddEntry("a.example.com", net.ParseIP("10.0.0.1"), "pod")
	table.AddEntry("b.example.com", net.ParseIP("10.0.0.2"), "pod")

	table.Clear()

	if table.Count() != 0 {
		t.Errorf("expected count 0 after clear, got %d", table.Count())
	}
}

func TestTable_ConcurrentAccess(t *testing.T) {
	table := NewTable()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ip := net.IPv4(10, 0, byte(i/256), byte(i%256))
			table.AddEntry("test.example.com", ip, "pod")
		}(i)
	}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			table.Lookup("test.example.com")
			table.LookupRandom("test.example.com")
		}()
	}

	wg.Wait()

	if table.Count() != 100 {
		t.Errorf("expected 100 entries, got %d", table.Count())
	}
}

func TestTable_IPv6Entry(t *testing.T) {
	table := NewTable()

	ipv6 := net.ParseIP("2001:db8::1")
	table.AddEntry("test.example.com", ipv6, "pod")

	ips := table.Lookup("test.example.com")
	if len(ips) != 1 {
		t.Errorf("expected 1 IP, got %d", len(ips))
	}
	if !ips[0].Equal(ipv6) {
		t.Errorf("expected IP %v, got %v", ipv6, ips[0])
	}
}

func TestTable_LookupIPv4(t *testing.T) {
	table := NewTable()

	ipv4 := net.ParseIP("10.0.0.1")
	ipv6 := net.ParseIP("2001:db8::1")

	table.AddEntry("test.example.com", ipv4, "pod")
	table.AddEntry("test.example.com", ipv6, "pod")

	ipv4s := table.LookupIPv4("test.example.com")
	if len(ipv4s) != 1 {
		t.Errorf("expected 1 IPv4 address, got %d", len(ipv4s))
	}
	if !ipv4s[0].Equal(ipv4) {
		t.Errorf("expected IPv4 %v, got %v", ipv4, ipv4s[0])
	}
}

func TestTable_LookupIPv6(t *testing.T) {
	table := NewTable()

	ipv4 := net.ParseIP("10.0.0.1")
	ipv6 := net.ParseIP("2001:db8::1")

	table.AddEntry("test.example.com", ipv4, "pod")
	table.AddEntry("test.example.com", ipv6, "pod")

	ipv6s := table.LookupIPv6("test.example.com")
	if len(ipv6s) != 1 {
		t.Errorf("expected 1 IPv6 address, got %d", len(ipv6s))
	}
	if !ipv6s[0].Equal(ipv6) {
		t.Errorf("expected IPv6 %v, got %v", ipv6, ipv6s[0])
	}
}

func TestTable_LookupRandomIPv4(t *testing.T) {
	table := NewTable()

	table.AddEntry("test.example.com", net.ParseIP("10.0.0.1"), "pod")
	table.AddEntry("test.example.com", net.ParseIP("10.0.0.2"), "pod")
	table.AddEntry("test.example.com", net.ParseIP("2001:db8::1"), "pod")
	table.AddEntry("test.example.com", net.ParseIP("2001:db8::2"), "pod")

	for i := 0; i < 20; i++ {
		ip := table.LookupRandomIPv4("test.example.com")
		if ip == nil {
			t.Fatal("expected non-nil IP from LookupRandomIPv4")
		}
		if ip.To4() == nil {
			t.Errorf("LookupRandomIPv4 returned non-IPv4 address: %v", ip)
		}
	}
}

func TestTable_LookupRandomIPv6(t *testing.T) {
	table := NewTable()

	table.AddEntry("test.example.com", net.ParseIP("10.0.0.1"), "pod")
	table.AddEntry("test.example.com", net.ParseIP("10.0.0.2"), "pod")
	table.AddEntry("test.example.com", net.ParseIP("2001:db8::1"), "pod")
	table.AddEntry("test.example.com", net.ParseIP("2001:db8::2"), "pod")

	for i := 0; i < 20; i++ {
		ip := table.LookupRandomIPv6("test.example.com")
		if ip == nil {
			t.Fatal("expected non-nil IP from LookupRandomIPv6")
		}
		if ip.To4() != nil {
			t.Errorf("LookupRandomIPv6 returned IPv4 address: %v", ip)
		}
	}
}

func TestTable_LookupIPv4Only(t *testing.T) {
	table := NewTable()

	table.AddEntry("test.example.com", net.ParseIP("2001:db8::1"), "pod")

	ipv4s := table.LookupIPv4("test.example.com")
	if len(ipv4s) != 0 {
		t.Errorf("expected 0 IPv4 addresses, got %d", len(ipv4s))
	}

	ip := table.LookupRandomIPv4("test.example.com")
	if ip != nil {
		t.Errorf("expected nil for LookupRandomIPv4 with only IPv6 entries, got %v", ip)
	}
}

func TestTable_LookupIPv6Only(t *testing.T) {
	table := NewTable()

	table.AddEntry("test.example.com", net.ParseIP("10.0.0.1"), "pod")

	ipv6s := table.LookupIPv6("test.example.com")
	if len(ipv6s) != 0 {
		t.Errorf("expected 0 IPv6 addresses, got %d", len(ipv6s))
	}

	ip := table.LookupRandomIPv6("test.example.com")
	if ip != nil {
		t.Errorf("expected nil for LookupRandomIPv6 with only IPv4 entries, got %v", ip)
	}
}

func TestTable_MixedIPVersions(t *testing.T) {
	table := NewTable()

	table.AddEntry("test.example.com", net.ParseIP("10.0.0.1"), "pod")
	table.AddEntry("test.example.com", net.ParseIP("10.0.0.2"), "service")
	table.AddEntry("test.example.com", net.ParseIP("2001:db8::1"), "pod")
	table.AddEntry("test.example.com", net.ParseIP("2001:db8::2"), "service")

	if table.Count() != 4 {
		t.Errorf("expected count 4, got %d", table.Count())
	}

	ipv4s := table.LookupIPv4("test.example.com")
	if len(ipv4s) != 2 {
		t.Errorf("expected 2 IPv4 addresses, got %d", len(ipv4s))
	}

	ipv6s := table.LookupIPv6("test.example.com")
	if len(ipv6s) != 2 {
		t.Errorf("expected 2 IPv6 addresses, got %d", len(ipv6s))
	}
}
