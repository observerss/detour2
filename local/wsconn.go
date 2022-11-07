package local

import (
	"detour/common"
	"errors"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	TIME_TO_LIVE       = 56 // sec
	RECONNECT_INTERVAL = 1  // sec
	FLUSH_TIMEOUT      = 50 // ms
)

type WSConn struct {
	Url        string
	Wid        string
	TimeToLive int
	CanConnect bool
	Connected  bool
	WSConn     *websocket.Conn
	WriteLock  sync.Mutex
	RWLock     sync.RWMutex
	Packer     *common.Packer
	Local      *Local
}

func NewWSConn(url string, wid string, local *Local) *WSConn {
	return &WSConn{
		Url:        strings.TrimSpace(url),
		Wid:        wid,
		TimeToLive: TIME_TO_LIVE,
		Connected:  false,
		CanConnect: true,
		Packer:     local.Packer,
		Local:      local,
	}
}

// GetWSConn find one usable wsconn
func (l *Local) GetWSConn() (*WSConn, error) {
	length := len(l.WSConns)
	wsconns := make([]*WSConn, 0, length)
	for _, w := range l.WSConns {
		wsconns = append(wsconns, w)
	}
	visited := make([]byte, length)
	for i := 0; i < length; i++ {
		visited = append(visited, 0)
	}

	for {
		if Sum(visited) == length {
			return nil, errors.New("all wsconns are not reachable")
		}

		idx := rand.Intn(length)
		if visited[idx] != 0 {
			continue
		}
		wsconn := wsconns[idx]
		if !wsconn.CanConnect {
			visited[idx] = 1
			continue
		}

		if !wsconn.Connected {
			err := Connect(wsconn, false)
			if err != nil {
				visited[idx] = 1
				continue
			}
		}

		return wsconn, nil
	}
}

func Connect(wsconn *WSConn, force bool) error {
	wsconn.RWLock.RLock()
	if !wsconn.CanConnect && !force {
		wsconn.RWLock.Unlock()
		return errors.New("can not connect")
	}
	wsconn.RWLock.RUnlock()

	dialer := websocket.Dialer{HandshakeTimeout: time.Second * 3}
	conn, _, err := dialer.Dial(wsconn.Url, nil)

	wsconn.RWLock.Lock()
	if err != nil {
		wsconn.CanConnect = false
		wsconn.Connected = false
		return err
	}
	log.Println(wsconn.Wid, "ws, connected")
	wsconn.Connected = true
	wsconn.CanConnect = true
	wsconn.WSConn = conn
	wsconn.RWLock.Unlock()

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
	var switchTimer *time.Timer

	log.Println(ws.Wid, "ws, start")
	for {
		// FIXIT: this will keep server running, not we want
		// try connect if not connected
		if !ws.Connected {
			log.Println(ws.Wid, "ws, wait for reconnect")
			time.Sleep(time.Second * RECONNECT_INTERVAL)
			err := Connect(ws, true)
			if err != nil {
				continue
			}
			switchTimer = time.NewTimer(time.Second * time.Duration(ws.TimeToLive))
		}

		// switch conn when ttl is reached
		if switchTimer != nil {
			select {
			case <-switchTimer.C:
				// create new connection
				log.Println(ws.Wid, "ws, switch start")
				wsconn := NewWSConn(ws.Url, ws.Wid, ws.Local)
				err := Connect(wsconn, false)
				if err != nil {
					// use old
					break
				}

				// send switch cmd (new wsconn)
				err = wsconn.WriteMessage(&common.Message{
					Wid: ws.Wid,
					Cmd: common.SWITCH,
				})
				if err != nil {
					// use old
					break
				}

				// flush all reads (old ws)
				ws.WSConn.SetReadDeadline(time.Now().Add(time.Millisecond * FLUSH_TIMEOUT))
				for {
					msg, err := ws.ReadMessage()
					if err != nil {
						break
					}
					log.Println(ws.Wid, "ws, flush read", msg.Cmd, len(msg.Data))
					conn, ok := ws.Local.Conns.Load(msg.Cid)
					if ok {
						log.Println(msg.Cid, "ws, put ===> queue", msg.Cmd, len(msg.Data))
						conn.(*Conn).MsgChan <- msg
					}
				}

				// finally do the switch
				ws.WriteLock.Lock()
				ws.WSConn.Close()
				// reset connection status, because
				ws.Connected = true
				ws.CanConnect = true
				ws.WSConn = wsconn.WSConn
				ws.WriteLock.Unlock()
			default:
			}
		}

		// read & put queue
		log.Println(ws.Wid, "ws, wait read")
		msg, err := ws.ReadMessage()
		if err != nil {
			log.Println(ws.Wid, "ws, read error", err)
			continue
		}

		log.Println(ws.Wid, "ws, read", msg.Cmd, len(msg.Data))
		conn, ok := ws.Local.Conns.Load(msg.Cid)
		if ok {
			log.Println(msg.Cid, "ws, put ===> queue", msg.Cmd, len(msg.Data))
			conn.(*Conn).MsgChan <- msg
		}
	}
}

func (ws *WSConn) WriteMessage(msg *common.Message) error {
	data, err := ws.Packer.Pack(msg)
	if err != nil {
		return err
	}
	ws.WriteLock.Lock()
	err = ws.WSConn.WriteMessage(websocket.BinaryMessage, data)
	ws.RWLock.Lock()
	if err != nil {
		log.Println("-----> set false by writemessage")
		ws.Connected = false
		ws.CanConnect = false
	}
	ws.RWLock.Unlock()
	ws.WriteLock.Unlock()
	return err
}

func (ws *WSConn) ReadMessage() (*common.Message, error) {
	for {
		mt, data, err := ws.WSConn.ReadMessage()
		if err != nil {
			ws.Connected = false
			ws.CanConnect = false
			return nil, err
		}
		if mt != websocket.BinaryMessage {
			continue
		}
		msg, err := ws.Packer.Unpack(data)
		if err != nil {
			return nil, err
		}
		return msg, nil
	}
}