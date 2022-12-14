package local

import (
	"errors"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/observerss/detour2/common"
	"github.com/observerss/detour2/logger"

	"github.com/gorilla/websocket"
)

const (
	DIAL_TIMEOUT       = 3   // sec
	TIME_TO_LIVE       = 596 // sec
	RECONNECT_INTERVAL = 1   // sec
	FLUSH_TIMEOUT      = 50  // ms
	INACTIVE_TIMEOUT   = 600 // sec
)

type WSConn struct {
	Url         string
	Wid         string
	TimeToLive  int
	CanConnect  bool
	Connected   bool
	ConnChan    chan interface{}
	WSConn      *websocket.Conn
	WriteLock   sync.Mutex
	RWLock      sync.RWMutex
	ConnectLock sync.Mutex
	Packer      *common.Packer
	Local       *Local
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
		ConnChan:   make(chan interface{}),
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
				logger.Error.Println("ws, connect error", err)
				visited[idx] = 1
				continue
			}
		}

		return wsconn, nil
	}
}

func Connect(wsconn *WSConn, force bool) error {
	logger.Debug.Println(wsconn.Wid, "ws, try connect")

	wsconn.ConnectLock.Lock()
	defer func() {
		wsconn.ConnectLock.Unlock()
		logger.Debug.Println(wsconn.Wid, "ws, try connect done")
	}()

	if wsconn.Connected {
		logger.Debug.Println(wsconn.Wid, "ws, connected by others")
		return nil
	}

	wsconn.RWLock.RLock()
	if !wsconn.CanConnect && !force {
		wsconn.RWLock.RUnlock()

		// let the blocked websocket puller try force connect
		if !IsClosed(wsconn.ConnChan) {
			close(wsconn.ConnChan)
		}
		return errors.New("can not connect")
	}
	wsconn.RWLock.RUnlock()

	dialer := websocket.Dialer{HandshakeTimeout: time.Second * DIAL_TIMEOUT}
	conn, _, err := dialer.Dial(wsconn.Url, nil)

	if err != nil {
		wsconn.RWLock.Lock()
		wsconn.CanConnect = false
		wsconn.Connected = false
		wsconn.RWLock.Unlock()
		logger.Debug.Println(wsconn.Wid, "ws, dial error", err)
		return err
	}
	wsconn.RWLock.Lock()
	logger.Debug.Println(wsconn.Wid, "ws, connected")
	wsconn.Connected = true
	wsconn.CanConnect = true
	wsconn.RWLock.Unlock()

	wsconn.WriteLock.Lock()
	wsconn.WSConn = conn
	wsconn.WriteLock.Unlock()

	if !IsClosed(wsconn.ConnChan) {
		close(wsconn.ConnChan)
	}
	return nil
}

func IsClosed(ch <-chan interface{}) bool {
	select {
	case <-ch:
		return true
	default:
	}

	return false
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

	logger.Debug.Println(ws.Wid, "ws, start")
	for {
		// block when num of conns are 0
		numOfConns := 0
		ws.Local.Conns.Range(func(key, value any) bool {
			tmpconn := value.(*Conn)
			tmpconn.AttrLock.RLock()
			if tmpconn.WSConn == ws && time.Since(tmpconn.LastActTime) < time.Second*INACTIVE_TIMEOUT {
				numOfConns += 1
			}
			tmpconn.AttrLock.RUnlock()
			return true
		})
		if numOfConns == 0 {
			ws.RWLock.Lock()
			if IsClosed(ws.ConnChan) {
				ws.ConnChan = make(chan interface{})
			}
			ws.RWLock.Unlock()
			logger.Info.Println(ws.Wid, "ws, num of conns == 0, block on ConnChan")
			<-ws.ConnChan
			switchTimer = time.NewTimer(time.Second * time.Duration(ws.TimeToLive))
		}

		// try connect if not connected
		if !ws.Connected {
			logger.Debug.Println(ws.Wid, "ws, wait for reconnect", numOfConns)
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
				logger.Debug.Println(ws.Wid, "ws, switch start")
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
					logger.Debug.Println(ws.Wid, "ws, flush read", msg.Cmd, len(msg.Data))
					conn, ok := ws.Local.Conns.Load(msg.Cid)
					if ok {
						logger.Debug.Println(msg.Cid, "ws, put ===> queue", msg.Cmd, len(msg.Data))
						conn.(*Conn).MsgChan <- msg
					}
				}

				// finally do the switch
				ws.WriteLock.Lock()
				ws.WSConn.Close()
				// reset connection status
				ws.RWLock.Lock()
				ws.Connected = true
				ws.RWLock.Unlock()
				ws.WSConn = wsconn.WSConn
				ws.WriteLock.Unlock()
			default:
			}
		}

		// read & put queue
		logger.Debug.Println(ws.Wid, "ws, wait read")
		msg, err := ws.ReadMessage()
		if err != nil {
			logger.Error.Println(ws.Wid, "ws, read error", err)
			continue
		}

		logger.Debug.Println(ws.Wid, "ws, read", msg.Cmd, len(msg.Data))
		conn, ok := ws.Local.Conns.Load(msg.Cid)
		if ok {
			logger.Debug.Println(msg.Cid, "ws, put ===> queue", msg.Cmd, len(msg.Data))
			conn.(*Conn).MsgChan <- msg
		} else {
			// no handler for this conn, should tell remote to stop
			logger.Debug.Println(msg.Cid, "ws, handler has quit, tell ws to close")
			msg.Cmd = common.CLOSE
			msg.Data = []byte{}
			ws.WriteMessage(msg)
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
		ws.Connected = false
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
