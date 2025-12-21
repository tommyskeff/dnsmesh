package dns

import (
	"fmt"
	"net"

	"github.com/miekg/dns"
	"github.com/tommyskeff/dnsmesh/internal/logging"
)

type Server struct {
	table  *Table
	ttl    uint32
	server *dns.Server
}

func NewServer(table *Table, ttl uint32) *Server {
	return &Server{
		table: table,
		ttl:   ttl,
	}
}

func (s *Server) Start(addr string) error {
	dns.HandleFunc(".", s.handleDNS)

	s.server = &dns.Server{
		Addr: addr,
		Net:  "udp",
	}

	logging.Info("Starting DNS server on %s", addr)
	return s.server.ListenAndServe()
}

func (s *Server) StartAsync(addr string) error {
	dns.HandleFunc(".", s.handleDNS)

	s.server = &dns.Server{
		Addr: addr,
		Net:  "udp",
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil {
			logging.Error("DNS server error: %v", err)
		}
	}()

	return nil
}

func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Shutdown()
	}
	return nil
}

func (s *Server) handleDNS(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	for _, q := range r.Question {
		switch q.Qtype {
		case dns.TypeA:
			s.handleA(m, q)
		case dns.TypeAAAA:
			s.handleAAAA(m, q)
		default:
		}
	}

	if len(m.Answer) == 0 && len(r.Question) > 0 {
		m.SetRcode(r, dns.RcodeNameError)
	}

	w.WriteMsg(m)
}

func (s *Server) handleA(m *dns.Msg, q dns.Question) {
	ip := s.table.LookupRandomIPv4(q.Name)
	if ip == nil {
		return
	}

	ipv4 := ip.To4()
	if ipv4 == nil {
		return
	}

	rr := &dns.A{
		Hdr: dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    s.ttl,
		},
		A: ipv4,
	}
	m.Answer = append(m.Answer, rr)
}

func (s *Server) handleAAAA(m *dns.Msg, q dns.Question) {
	ip := s.table.LookupRandomIPv6(q.Name)
	if ip == nil {
		return
	}

	ipv6 := ip.To16()
	if ipv6 == nil {
		return
	}

	rr := &dns.AAAA{
		Hdr: dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeAAAA,
			Class:  dns.ClassINET,
			Ttl:    s.ttl,
		},
		AAAA: ipv6,
	}
	m.Answer = append(m.Answer, rr)
}

func (s *Server) GetTable() *Table {
	return s.table
}

func (s *Server) SetTTL(ttl uint32) {
	s.ttl = ttl
}

func (s *Server) Handler() dns.HandlerFunc {
	return s.handleDNS
}

func (s *Server) QueryLocal(name string, qtype uint16) (*dns.Msg, error) {
	if s.server == nil {
		return nil, fmt.Errorf("server not started")
	}

	c := new(dns.Client)
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)

	r, _, err := c.Exchange(m, s.server.Addr)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Server) HandleTestQuery(name string, qtype uint16) *dns.Msg {
	r := new(dns.Msg)
	r.SetQuestion(dns.Fqdn(name), qtype)

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	for _, q := range r.Question {
		switch q.Qtype {
		case dns.TypeA:
			s.handleA(m, q)
		case dns.TypeAAAA:
			s.handleAAAA(m, q)
		}
	}

	if len(m.Answer) == 0 && len(r.Question) > 0 {
		m.SetRcode(r, dns.RcodeNameError)
	}

	return m
}

type TestResponseWriter struct {
	msg        *dns.Msg
	localAddr  net.Addr
	remoteAddr net.Addr
}

func NewTestResponseWriter() *TestResponseWriter {
	return &TestResponseWriter{
		localAddr:  &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53},
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
	}
}

func (w *TestResponseWriter) LocalAddr() net.Addr  { return w.localAddr }
func (w *TestResponseWriter) RemoteAddr() net.Addr { return w.remoteAddr }
func (w *TestResponseWriter) WriteMsg(m *dns.Msg) error {
	w.msg = m
	return nil
}
func (w *TestResponseWriter) Write([]byte) (int, error) { return 0, nil }
func (w *TestResponseWriter) Close() error              { return nil }
func (w *TestResponseWriter) TsigStatus() error         { return nil }
func (w *TestResponseWriter) TsigTimersOnly(bool)       {}
func (w *TestResponseWriter) Hijack()                   {}
func (w *TestResponseWriter) GetMsg() *dns.Msg          { return w.msg }
