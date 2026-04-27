package local

import (
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/observerss/detour2/common"
)

func newConnectedTestWSConn(local *Local, wid string, active int64) *WSConn {
	wsconn := NewWSConn("ws://127.0.0.1/ws", wid, local)
	wsconn.Connected = true
	wsconn.WSConn = &websocket.Conn{}
	wsconn.AddActive(active)
	return wsconn
}

func TestGetWSConnWakesIdleConnectedWSConn(t *testing.T) {
	local := &Local{
		Packer:  &common.Packer{Password: "pass123"},
		WSConns: make(map[string]*WSConn),
	}
	wsconn := newConnectedTestWSConn(local, "wid", 0)
	local.WSConns["one"] = wsconn

	got, err := local.GetWSConn()
	if err != nil {
		t.Fatal(err)
	}
	if got != wsconn {
		t.Fatal("unexpected wsconn selected")
	}
	if !isClosed(wsconn.ConnChan) {
		t.Fatal("expected idle puller channel to be signaled")
	}
	if wsconn.ActiveCount() != 1 {
		t.Fatalf("unexpected active count: %d", wsconn.ActiveCount())
	}
}

func TestGetWSConnSelectsLeastActiveWSConn(t *testing.T) {
	local := &Local{
		Packer:  &common.Packer{Password: "pass123"},
		WSConns: make(map[string]*WSConn),
	}
	busy := newConnectedTestWSConn(local, "busy", 3)
	idle := newConnectedTestWSConn(local, "idle", 1)
	local.WSConns["busy"] = busy
	local.WSConns["idle"] = idle

	got, err := local.GetWSConn()
	if err != nil {
		t.Fatal(err)
	}
	if got != idle {
		t.Fatalf("expected least active wsconn, got %s", got.Wid)
	}
	if idle.ActiveCount() != 2 {
		t.Fatalf("unexpected idle active count: %d", idle.ActiveCount())
	}
}

func TestStopLocalStopsIdleWebsocketPuller(t *testing.T) {
	localServer := &Local{
		Packer:  &common.Packer{Password: "pass123"},
		WSConns: make(map[string]*WSConn),
		Done:    make(chan struct{}),
	}
	wsconn := NewWSConn("ws://127.0.0.1/ws", "wid", localServer)
	localServer.WSConns["one"] = wsconn

	done := make(chan error, 1)
	go func() {
		done <- wsconn.WebsocketPuller()
	}()

	localServer.StopLocal()
	localServer.StopLocal()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("websocket puller did not stop")
	}
}
