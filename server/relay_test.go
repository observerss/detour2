package server

import (
	"sync"
	"testing"

	"github.com/observerss/detour2/common"

	"github.com/gorilla/websocket"
)

func TestHandleSwitchUpdatesRelayConnUpstream(t *testing.T) {
	oldWS := &websocket.Conn{}
	newWS := &websocket.Conn{}
	oldLock := &sync.Mutex{}
	newLock := &sync.Mutex{}
	relay := &RelayClient{Wid: "next"}
	server := &Server{}
	server.Conns.Store("cid", &Conn{
		Cid:    "cid",
		Wid:    "wid",
		WSConn: oldWS,
		WSLock: oldLock,
		Relay:  relay,
	})

	server.HandleSwitch(&Handle{
		WSConn: newWS,
		WSLock: newLock,
		Msg:    &common.Message{Wid: "wid"},
	})

	value, ok := server.Conns.Load("cid")
	if !ok {
		t.Fatal("connection was removed")
	}
	conn := value.(*Conn)
	if conn.WSConn != newWS {
		t.Fatal("websocket was not switched")
	}
	if conn.WSLock != newLock {
		t.Fatal("websocket lock was not switched")
	}
	if conn.Relay != relay {
		t.Fatal("relay client should be preserved")
	}
}
