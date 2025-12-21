package dns

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

func TestServer_HandleA_Found(t *testing.T) {
	table := NewTable()
	table.AddEntry("pricing-server.prod.internal", net.ParseIP("10.0.0.1"), "pod")

	server := NewServer(table, 30)

	m := server.HandleTestQuery("pricing-server.prod.internal", dns.TypeA)

	if m.Rcode != dns.RcodeSuccess {
		t.Errorf("expected NOERROR, got %s", dns.RcodeToString[m.Rcode])
	}

	if len(m.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(m.Answer))
	}

	a, ok := m.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("expected A record, got %T", m.Answer[0])
	}

	if !a.A.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("expected IP 10.0.0.1, got %v", a.A)
	}

	if a.Hdr.Ttl != 30 {
		t.Errorf("expected TTL 30, got %d", a.Hdr.Ttl)
	}
}

func TestServer_HandleA_NotFound(t *testing.T) {
	table := NewTable()
	server := NewServer(table, 30)

	m := server.HandleTestQuery("nonexistent.prod.internal", dns.TypeA)

	if m.Rcode != dns.RcodeNameError {
		t.Errorf("expected NXDOMAIN, got %s", dns.RcodeToString[m.Rcode])
	}

	if len(m.Answer) != 0 {
		t.Errorf("expected 0 answers for NXDOMAIN, got %d", len(m.Answer))
	}
}

func TestServer_HandleA_MultipleIPs(t *testing.T) {
	table := NewTable()
	table.AddEntry("pricing-server.prod.internal", net.ParseIP("10.0.0.1"), "pod")
	table.AddEntry("pricing-server.prod.internal", net.ParseIP("10.0.0.2"), "pod")
	table.AddEntry("pricing-server.prod.internal", net.ParseIP("10.0.0.3"), "pod")

	server := NewServer(table, 30)

	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		m := server.HandleTestQuery("pricing-server.prod.internal", dns.TypeA)

		if len(m.Answer) != 1 {
			t.Fatalf("expected 1 answer, got %d", len(m.Answer))
		}

		a := m.Answer[0].(*dns.A)
		seen[a.A.String()] = true
	}

	if len(seen) < 2 {
		t.Logf("warning: only saw %d unique IPs in 50 queries", len(seen))
	}
}

func TestServer_HandleA_CaseInsensitive(t *testing.T) {
	table := NewTable()
	table.AddEntry("pricing-server.prod.internal", net.ParseIP("10.0.0.1"), "pod")

	server := NewServer(table, 30)

	m := server.HandleTestQuery("PRICING-SERVER.PROD.INTERNAL", dns.TypeA)

	if m.Rcode != dns.RcodeSuccess {
		t.Errorf("expected NOERROR for case-insensitive query, got %s", dns.RcodeToString[m.Rcode])
	}

	if len(m.Answer) != 1 {
		t.Errorf("expected 1 answer for case-insensitive query, got %d", len(m.Answer))
	}
}

func TestServer_HandleOtherTypes(t *testing.T) {
	table := NewTable()
	table.AddEntry("pricing-server.prod.internal", net.ParseIP("10.0.0.1"), "pod")

	server := NewServer(table, 30)

	m := server.HandleTestQuery("pricing-server.prod.internal", dns.TypeMX)

	if len(m.Answer) != 0 {
		t.Errorf("expected 0 answers for unsupported type, got %d", len(m.Answer))
	}
}

func TestServer_SetTTL(t *testing.T) {
	table := NewTable()
	table.AddEntry("test.example.com", net.ParseIP("10.0.0.1"), "pod")

	server := NewServer(table, 30)
	server.SetTTL(60)

	m := server.HandleTestQuery("test.example.com", dns.TypeA)

	if len(m.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(m.Answer))
	}

	a := m.Answer[0].(*dns.A)
	if a.Hdr.Ttl != 60 {
		t.Errorf("expected TTL 60 after SetTTL, got %d", a.Hdr.Ttl)
	}
}

func TestServer_HandlerFunc(t *testing.T) {
	table := NewTable()
	table.AddEntry("test.example.com", net.ParseIP("10.0.0.1"), "pod")

	server := NewServer(table, 30)

	w := NewTestResponseWriter()
	r := new(dns.Msg)
	r.SetQuestion(dns.Fqdn("test.example.com"), dns.TypeA)

	server.Handler()(w, r)

	m := w.GetMsg()
	if m == nil {
		t.Fatal("expected response message")
	}

	if len(m.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(m.Answer))
	}
}

func TestServer_HandleAAAA_Found(t *testing.T) {
	table := NewTable()
	table.AddEntry("pricing-server.prod.internal", net.ParseIP("2001:db8::1"), "pod")

	server := NewServer(table, 30)

	m := server.HandleTestQuery("pricing-server.prod.internal", dns.TypeAAAA)

	if m.Rcode != dns.RcodeSuccess {
		t.Errorf("expected NOERROR, got %s", dns.RcodeToString[m.Rcode])
	}

	if len(m.Answer) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(m.Answer))
	}

	aaaa, ok := m.Answer[0].(*dns.AAAA)
	if !ok {
		t.Fatalf("expected AAAA record, got %T", m.Answer[0])
	}

	expectedIP := net.ParseIP("2001:db8::1")
	if !aaaa.AAAA.Equal(expectedIP) {
		t.Errorf("expected IP %v, got %v", expectedIP, aaaa.AAAA)
	}

	if aaaa.Hdr.Ttl != 30 {
		t.Errorf("expected TTL 30, got %d", aaaa.Hdr.Ttl)
	}
}

func TestServer_HandleAAAA_NotFound(t *testing.T) {
	table := NewTable()
	server := NewServer(table, 30)

	m := server.HandleTestQuery("nonexistent.prod.internal", dns.TypeAAAA)

	if m.Rcode != dns.RcodeNameError {
		t.Errorf("expected NXDOMAIN, got %s", dns.RcodeToString[m.Rcode])
	}

	if len(m.Answer) != 0 {
		t.Errorf("expected 0 answers for NXDOMAIN, got %d", len(m.Answer))
	}
}

func TestServer_HandleAAAA_OnlyIPv4Exists(t *testing.T) {
	table := NewTable()
	table.AddEntry("pricing-server.prod.internal", net.ParseIP("10.0.0.1"), "pod")

	server := NewServer(table, 30)

	m := server.HandleTestQuery("pricing-server.prod.internal", dns.TypeAAAA)

	if len(m.Answer) != 0 {
		t.Errorf("expected 0 answers when only IPv4 exists, got %d", len(m.Answer))
	}
}

func TestServer_HandleA_OnlyIPv6Exists(t *testing.T) {
	table := NewTable()
	table.AddEntry("pricing-server.prod.internal", net.ParseIP("2001:db8::1"), "pod")

	server := NewServer(table, 30)

	m := server.HandleTestQuery("pricing-server.prod.internal", dns.TypeA)

	if len(m.Answer) != 0 {
		t.Errorf("expected 0 answers when only IPv6 exists, got %d", len(m.Answer))
	}
}

func TestServer_HandleMixedIPVersions(t *testing.T) {
	table := NewTable()
	table.AddEntry("dual-stack.prod.internal", net.ParseIP("10.0.0.1"), "pod")
	table.AddEntry("dual-stack.prod.internal", net.ParseIP("2001:db8::1"), "pod")

	server := NewServer(table, 30)

	m := server.HandleTestQuery("dual-stack.prod.internal", dns.TypeA)
	if m.Rcode != dns.RcodeSuccess {
		t.Errorf("expected NOERROR for A query, got %s", dns.RcodeToString[m.Rcode])
	}
	if len(m.Answer) != 1 {
		t.Fatalf("expected 1 A answer, got %d", len(m.Answer))
	}
	a, ok := m.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("expected A record, got %T", m.Answer[0])
	}
	if !a.A.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("expected A record with 10.0.0.1, got %v", a.A)
	}

	m = server.HandleTestQuery("dual-stack.prod.internal", dns.TypeAAAA)
	if m.Rcode != dns.RcodeSuccess {
		t.Errorf("expected NOERROR for AAAA query, got %s", dns.RcodeToString[m.Rcode])
	}
	if len(m.Answer) != 1 {
		t.Fatalf("expected 1 AAAA answer, got %d", len(m.Answer))
	}
	aaaa, ok := m.Answer[0].(*dns.AAAA)
	if !ok {
		t.Fatalf("expected AAAA record, got %T", m.Answer[0])
	}
	if !aaaa.AAAA.Equal(net.ParseIP("2001:db8::1")) {
		t.Errorf("expected AAAA record with 2001:db8::1, got %v", aaaa.AAAA)
	}
}

func TestServer_HandleAAAA_MultipleIPs(t *testing.T) {
	table := NewTable()
	table.AddEntry("multi-ipv6.prod.internal", net.ParseIP("2001:db8::1"), "pod")
	table.AddEntry("multi-ipv6.prod.internal", net.ParseIP("2001:db8::2"), "pod")
	table.AddEntry("multi-ipv6.prod.internal", net.ParseIP("2001:db8::3"), "pod")

	server := NewServer(table, 30)

	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		m := server.HandleTestQuery("multi-ipv6.prod.internal", dns.TypeAAAA)

		if len(m.Answer) != 1 {
			t.Fatalf("expected 1 answer, got %d", len(m.Answer))
		}

		aaaa := m.Answer[0].(*dns.AAAA)
		seen[aaaa.AAAA.String()] = true
	}

	if len(seen) < 2 {
		t.Logf("warning: only saw %d unique IPs in 50 queries", len(seen))
	}
}
