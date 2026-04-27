package local

import (
	"net"
	"sync"
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

func TestDeliverMessageClosesSlowConnWhenQueueBlocked(t *testing.T) {
	localServer := &Local{
		Packer:  &common.Packer{Password: "pass123"},
		WSConns: make(map[string]*WSConn),
		Conns:   syncMap(),
		Done:    make(chan struct{}),
		Metrics: common.NewRuntimeMetrics(),
	}
	oldTimeout := msgQueueTimeout
	msgQueueTimeout = 10 * time.Millisecond
	t.Cleanup(func() { msgQueueTimeout = oldTimeout })

	wsconn := NewWSConn("ws://127.0.0.1/ws", "wid", localServer)
	wsconn.AddActive(1)
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	conn := &Conn{
		Cid:     "slow",
		Wid:     wsconn.Wid,
		MsgChan: make(chan *common.Message, 1),
		Quit:    make(chan interface{}),
		NetConn: serverConn,
		WSConn:  wsconn,
		Metrics: localServer.Metrics,
	}
	conn.MsgChan <- &common.Message{Cid: conn.Cid, Wid: conn.Wid, Cmd: common.DATA}
	localServer.Conns.Store(conn.Cid, conn)

	if wsconn.DeliverMessage(conn, &common.Message{Cid: conn.Cid, Wid: conn.Wid, Cmd: common.DATA}) {
		t.Fatal("expected blocked delivery to close the slow conn")
	}
	if got := localServer.Metrics.Snapshot().QueueTimeoutsTotal; got != 1 {
		t.Fatalf("unexpected queue timeout count: %d", got)
	}
	if wsconn.ActiveCount() != 0 {
		t.Fatalf("unexpected active count: %d", wsconn.ActiveCount())
	}
	if _, ok := localServer.Conns.Load(conn.Cid); ok {
		t.Fatal("slow conn was not removed")
	}
	select {
	case <-conn.Quit:
	case <-time.After(time.Second):
		t.Fatal("slow conn quit channel was not closed")
	}
}

func syncMap() sync.Map {
	return sync.Map{}
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
