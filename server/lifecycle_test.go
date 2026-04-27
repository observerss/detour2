package server

import (
	"net"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/observerss/detour2/common"
)

func TestCloseRelayConnsReleasesActiveAndClosesUpstream(t *testing.T) {
	server := NewServer(&common.ServerConfig{
		Listen:   "tcp://127.0.0.1:3811",
		Password: "pass123",
	})
	relay := NewRelayClient("ws://127.0.0.1:3812/ws", "relay-wid", server)
	relay.AddActive(1)

	written := make(chan *common.Message, 1)
	writer := common.NewFairMessageWriter(func(msg *common.Message) error {
		written <- common.CloneMessage(msg)
		return nil
	}, common.DefaultMessageQueueLimit)
	defer writer.Close()

	server.Conns.Store("cid", &Conn{
		Cid:      "cid",
		Wid:      "upstream-wid",
		Network:  "tcp",
		Address:  "example.com:443",
		WSWriter: writer,
		Relay:    relay,
	})

	server.CloseRelayConns(relay)

	if relay.ActiveCount() != 0 {
		t.Fatalf("unexpected relay active count: %d", relay.ActiveCount())
	}
	if _, ok := server.Conns.Load("cid"); ok {
		t.Fatal("relay conn was not removed")
	}
	select {
	case msg := <-written:
		if msg.Cmd != common.CLOSE || msg.Cid != "cid" || msg.Wid != "upstream-wid" {
			t.Fatalf("unexpected close message: %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("upstream close message was not written")
	}
}

func TestCloseWebsocketConnsClosesAssociatedConnections(t *testing.T) {
	server := NewServer(&common.ServerConfig{
		Listen:   "tcp://127.0.0.1:3811",
		Password: "pass123",
	})
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	wsconn := &websocket.Conn{}
	writer := common.NewFairMessageWriter(func(msg *common.Message) error { return nil }, common.DefaultMessageQueueLimit)
	defer writer.Close()

	server.Conns.Store("cid", &Conn{
		Cid:      "cid",
		Wid:      "wid",
		WSConn:   wsconn,
		WSWriter: writer,
		NetConn:  serverConn,
	})

	server.CloseWebsocketConns(wsconn, writer)

	if _, ok := server.Conns.Load("cid"); ok {
		t.Fatal("websocket conn was not removed")
	}
	if _, err := serverConn.Write([]byte("x")); err == nil {
		t.Fatal("expected associated net conn to be closed")
	}
}

func TestRunLoopSendsCloseOnTargetReadError(t *testing.T) {
	server := NewServer(&common.ServerConfig{
		Listen:   "tcp://127.0.0.1:3811",
		Password: "pass123",
	})
	clientConn, serverConn := net.Pipe()

	written := make(chan *common.Message, 1)
	writer := common.NewFairMessageWriter(func(msg *common.Message) error {
		written <- common.CloneMessage(msg)
		return nil
	}, common.DefaultMessageQueueLimit)
	defer writer.Close()

	conn := &Conn{
		Cid:      "cid",
		Wid:      "wid",
		Network:  "tcp",
		Address:  "example.com:443",
		NetConn:  serverConn,
		WSWriter: writer,
	}
	server.Conns.Store(conn.Cid, conn)

	done := make(chan struct{})
	go func() {
		server.RunLoop(conn)
		close(done)
	}()

	if err := clientConn.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-written:
		if msg.Cmd != common.CLOSE || msg.Cid != conn.Cid || msg.Wid != conn.Wid {
			t.Fatalf("unexpected close message: %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("target read error did not send upstream close")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("run loop did not exit")
	}
	if _, ok := server.Conns.Load(conn.Cid); ok {
		t.Fatal("conn was not removed")
	}
}
