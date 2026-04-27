package server

import (
	"testing"

	"github.com/observerss/detour2/common"
)

func TestServerMetricsSnapshot(t *testing.T) {
	server := NewServer(&common.ServerConfig{
		Listen:        "tcp://127.0.0.1:3811",
		Remotes:       "ws://127.0.0.1:3812/ws,ws://127.0.0.1:3813/ws",
		Password:      "pass123",
		RelayPoolSize: 1,
		DNSServers:    "8.8.8.8:53",
	})
	server.Conns.Store("cid", &Conn{Cid: "cid", Wid: "upstream"})
	server.Metrics.MessagesInTotal.Inc()

	for _, relay := range server.RelayClients {
		relay.SetConnected(true)
		relay.AddActive(3)
		break
	}

	snapshot := server.MetricsSnapshot()
	if snapshot.Role != "relay" {
		t.Fatalf("unexpected role: %s", snapshot.Role)
	}
	if snapshot.Listen != "127.0.0.1:3811" {
		t.Fatalf("unexpected listen address: %s", snapshot.Listen)
	}
	if snapshot.Connections.Active != 1 || snapshot.Connections.ByWID["upstream"] != 1 {
		t.Fatalf("unexpected connection summary: %+v", snapshot.Connections)
	}
	if snapshot.RelayPool.Total != 2 || snapshot.RelayPool.Connected != 1 {
		t.Fatalf("unexpected relay pool summary: %+v", snapshot.RelayPool)
	}
	if snapshot.RelayPool.ActiveTotal != 3 || snapshot.RelayPool.MaxActive != 3 {
		t.Fatalf("unexpected relay active counts: %+v", snapshot.RelayPool)
	}
	if snapshot.Runtime.MessagesInTotal != 1 {
		t.Fatalf("unexpected runtime counters: %+v", snapshot.Runtime)
	}
	if len(snapshot.DNSServers) != 1 || snapshot.DNSServers[0] != "8.8.8.8:53" {
		t.Fatalf("unexpected DNS servers: %+v", snapshot.DNSServers)
	}
}
