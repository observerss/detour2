package server

import (
	"detour/relay"

	"github.com/google/uuid"
)

const TIMEOUT = 60

type Tracker struct {
	Clients map[uuid.UUID]*relay.ClientInfo `json:"clients,omitempty" comment:"clientid => clientinfo"`
}

// Upsert ConnPair Info
// Note that a ConnPair can be already exists in Tracker
func (t *Tracker) Upsert(cp relay.ConnPair, conn *relay.ConnInfo) {
	// todo
}

func (t *Tracker) Find(cp relay.ConnPair) *relay.ConnInfo {
	// todo
	return nil
}

func (t *Tracker) Remove(cp relay.ConnPair) {
	// todo
}

func (t *Tracker) ImAlive(cp relay.ConnPair) {
	// clients, ok := t.Clients[cp.ClientId]
	// t.ClientActivities[id] = time.Now().UnixMilli()
}

// Housekeep the tracker
func (t *Tracker) RunHouseKeeper() {
	// now := time.Now().UnixMilli()
	// res := make([]uuid.UUID, 0)
	// for k, v := range t.ClientActivities {
	// 	if now-v > 1000*TIMEOUT {
	// 		if _, ok := t.ClientConns[k]; ok {
	// 			res = append(res, k)
	// 		}
	// 	}
	// }
	// return res
}

func NewTracker() *Tracker {
	return &Tracker{}
}
