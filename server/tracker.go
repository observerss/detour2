package server

import (
	"detour/relay"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

const ALIVE_TIMEOUT = 60

type Tracker struct {
	Clients map[uuid.UUID]*relay.ClientInfo `json:"clients,omitempty" comment:"clientid => clientinfo"`
	Lock    sync.Mutex
}

// Upsert ConnPair Info
// Note that a ConnPair can be already exists in Tracker
func (t *Tracker) Upsert(cp *relay.ConnPair, conn *relay.ConnInfo) *relay.ClientInfo {
	t.Lock.Lock()
	defer t.Lock.Unlock()
	client, ok := t.Clients[cp.ClientId]
	if !ok {
		conns := make(map[uuid.UUID]*relay.ConnInfo)
		conns[cp.ConnId] = conn
		client = &relay.ClientInfo{
			Conns:    conns,
			Activity: time.Now().UnixMilli(),
			Quit:     make(chan interface{}),
		}
		t.Clients[cp.ClientId] = client
	} else {
		oldconn, ok := client.Conns[cp.ConnId]
		if ok {
			close(oldconn.Quit)
		}
		client.Conns[cp.ConnId] = conn
	}
	client.Activity = time.Now().UnixMilli()
	return client
}

// Find ConnInfo by ConnPair
func (t *Tracker) Find(cp *relay.ConnPair) *relay.ConnInfo {
	t.Lock.Lock()
	defer t.Lock.Unlock()
	client, ok := t.Clients[cp.ClientId]
	if ok {
		conn, ok := client.Conns[cp.ConnId]
		if ok {
			return conn
		}
	}
	return nil
}

func (t *Tracker) Remove(cp *relay.ConnPair) {
	t.Lock.Lock()
	defer t.Lock.Unlock()
	client, ok := t.Clients[cp.ClientId]
	if ok {
		delete(client.Conns, cp.ConnId)
		if len(client.Conns) == 0 {
			delete(t.Clients, cp.ClientId)
		}
	}
}

func (t *Tracker) ImAlive(cp *relay.ConnPair) {
	t.Lock.Lock()
	defer t.Lock.Unlock()
	client, ok := t.Clients[cp.ClientId]
	if ok {
		client.Activity = time.Now().UnixMilli()
		conn, ok := client.Conns[cp.ConnId]
		if ok {
			conn.Activity = time.Now().UnixMilli()
		}
	}
}

// Housekeep the tracker
// this usually doesn't make much sense, unless a leak happened
func (t *Tracker) RunHouseKeeper() {
	t.Lock.Lock()
	defer t.Lock.Unlock()
	now := time.Now().UnixMilli()
	totalClients := 0
	totalConns := 0
	removedClients := 0
	removedConns := 0
	clientids := make([]uuid.UUID, 0)
	for clientid, client := range t.Clients {
		totalClients += 1
		connids := make([]uuid.UUID, 0)
		for connid, conn := range client.Conns {
			totalConns += 1
			if now-conn.Activity > 1000*ALIVE_TIMEOUT {
				removedConns += 1
				connids = append(connids, connid)
			}
		}
		for _, connid := range connids {
			delete(client.Conns, connid)
		}
		if len(client.Conns) == 0 {
			client.Quit <- nil
			removedClients += 1
			clientids = append(clientids, clientid)
		}
	}
	for _, clientid := range clientids {
		delete(t.Clients, clientid)
	}
	end := time.Now().UnixMilli()
	log.Println("house keep in", end-now, "ms",
		"totalClients=", totalClients, "totalConns=", totalConns,
		"removedClients=", removedClients, "removedCons=", removedConns)
}

func NewTracker() *Tracker {
	return &Tracker{Clients: make(map[uuid.UUID]*relay.ClientInfo)}
}
