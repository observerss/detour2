package local

import (
	"detour/common"
	"errors"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WSConn struct {
	Url        string
	Wid        string
	TimeToLive int
	CanConnect bool
	Connected  bool
	WSConn     *websocket.Conn
	ReadLock   sync.Mutex
	WriteLock  sync.Mutex
	Packer     *common.Packer
	Local      *Local
}

func NewWSConn(url string, wid string, local *Local) *WSConn {
	return &WSConn{
		Url:        strings.TrimSpace(url),
		Wid:        wid,
		TimeToLive: 58,
		Connected:  false,
		CanConnect: true,
		Packer:     local.Packer,
		Local:      local,
	}
}

// GetWSConn find one usable wsconn
func (l *Local) GetWSConn() (*WSConn, error) {
	length := len(l.WSConns)
	wsconns := make([]*WSConn, length)
	for _, w := range l.WSConns {
		wsconns = append(wsconns, w)
	}
	visited := make([]byte, length)
	for i := 0; i < length; i++ {
		visited = append(visited, 0)
	}

	for {
		idx := rand.Intn(length)
		if visited[idx] != 0 {
			continue
		}
		wsconn := wsconns[idx]
		err := Connect(wsconn)
		if err != nil {
			visited[idx] = 1
			if Sum(visited) == length {
				return nil, errors.New("all wsconns are not reachable")
			}
		} else {
			return wsconn, nil
		}
	}
}

func Connect(wsconn *WSConn) error {
	if !wsconn.CanConnect {
		return errors.New("can not connect")
	}
	dialer := websocket.Dialer{HandshakeTimeout: time.Second * 3}
	conn, _, err := dialer.Dial(wsconn.Url, nil)
	if err != nil {
		wsconn.CanConnect = false
		wsconn.Connected = false
		return err
	}
	wsconn.Connected = true
	wsconn.WSConn = conn
	return nil
}

func Sum(visited []byte) int {
	total := 0
	for _, v := range visited {
		total += int(v)
	}
	return total
}

// WebsocketPuller
// 1. pull message from websocket, put it to data channel
// 2. switch to a new transport connection when ttl is reached
func (ws *WSConn) WebsocketPuller() error {
	// time.NewTimer(time.Second * time.Duration(wsconn.TimeToLive))
	return nil
}

func (ws *WSConn) WriteMessage(msg *common.Message) error {
	data, err := ws.Packer.Pack(msg)
	if err != nil {
		return err
	}
	ws.WriteLock.Lock()
	defer ws.WriteLock.Unlock()
	return ws.WSConn.WriteMessage(websocket.BinaryMessage, data)
}
