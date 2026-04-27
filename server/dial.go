package server

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"time"
)

func ParseDNSServers(value string) []string {
	servers := []string{}
	for _, server := range strings.Split(value, ",") {
		server = strings.TrimSpace(server)
		if server == "" {
			continue
		}
		if _, _, err := net.SplitHostPort(server); err != nil {
			server = net.JoinHostPort(server, "53")
		}
		servers = append(servers, server)
	}
	return servers
}

func (s *Server) Dialer() net.Dialer {
	dialer := net.Dialer{Timeout: time.Second * DIAL_TIMEOUT}
	if len(s.DNSServers) == 0 {
		return dialer
	}
	dialer.Resolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network string, address string) (net.Conn, error) {
			idx := atomic.AddUint64(&s.DNSCounter, 1)
			server := s.DNSServers[int(idx-1)%len(s.DNSServers)]
			resolverDialer := net.Dialer{Timeout: time.Second * DIAL_TIMEOUT}
			return resolverDialer.DialContext(ctx, network, server)
		},
	}
	return dialer
}
