package server

import (
	"detour/schema"

	"github.com/google/uuid"
)

const TIMEOUT = 60

type Tracker struct {
	Clients map[uuid.UUID]*schema.ClientInfo `json:"clients,omitempty" comment:"clientid => clientinfo"`
}

func (t *Tracker) Add(cp schema.ConnPair, data schema.RelayData) {

}

func (t *Tracker) Remove(cp schema.ConnPair) {
	// todo
}

func (t *Tracker) ImAlive(cp schema.ConnPair) {
	// clients, ok := t.Clients[cp.ClientId]
	// t.ClientActivities[id] = time.Now().UnixMilli()
}

// get old clients and conns
func (t *Tracker) GetOldPairs() []schema.ConnPair {
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
	res := make([]schema.ConnPair, 0)
	return res
}

func NewTracker() *Tracker {
	return &Tracker{}
}
