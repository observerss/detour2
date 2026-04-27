package local

import (
	"testing"

	"github.com/observerss/detour2/common"
)

func TestLocalMetricsSnapshot(t *testing.T) {
	localServer := &Local{
		Network: "tcp",
		Address: "127.0.0.1:3810",
		Proto:   &HTTPProto{},
		Packer:  &common.Packer{Password: "pass123"},
		WSConns: make(map[string]*WSConn),
		Metrics: common.NewRuntimeMetrics(),
	}
	connected := newConnectedTestWSConn(localServer, "connected", 2)
	disconnected := NewWSConn("ws://127.0.0.1:3811/ws", "disconnected", localServer)
	disconnected.CanConnect = false

	localServer.WSConns["b"] = disconnected
	localServer.WSConns["a"] = connected
	localServer.Conns.Store("cid", &Conn{Cid: "cid", Wid: connected.Wid})
	localServer.Metrics.ClientConnectionsTotal.Inc()

	snapshot := localServer.MetricsSnapshot()
	if snapshot.Role != "local" {
		t.Fatalf("unexpected role: %s", snapshot.Role)
	}
	if snapshot.Connections.Active != 1 {
		t.Fatalf("unexpected active connections: %d", snapshot.Connections.Active)
	}
	if snapshot.WebSocketPool.Total != 2 || snapshot.WebSocketPool.Connected != 1 || snapshot.WebSocketPool.Connectable != 1 {
		t.Fatalf("unexpected websocket pool summary: %+v", snapshot.WebSocketPool)
	}
	if snapshot.WebSocketPool.ActiveTotal != 2 || snapshot.WebSocketPool.MaxActive != 2 {
		t.Fatalf("unexpected websocket active counts: %+v", snapshot.WebSocketPool)
	}
	if len(snapshot.WebSocketPool.Items) != 2 || snapshot.WebSocketPool.Items[0].Key != "a" {
		t.Fatalf("websocket items are not sorted: %+v", snapshot.WebSocketPool.Items)
	}
	if snapshot.Runtime.ClientConnectionsTotal != 1 {
		t.Fatalf("unexpected runtime counters: %+v", snapshot.Runtime)
	}
}
