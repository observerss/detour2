package local

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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
	Active      int64
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
	if l.IsStopped() {
		return nil, errors.New("local server is stopped")
	}
	wsconns := make([]*WSConn, 0, len(l.WSConns))
	for _, w := range l.WSConns {
		if w.CanConnectNow() {
			wsconns = append(wsconns, w)
		}
	}
	sort.SliceStable(wsconns, func(i, j int) bool {
		return wsconns[i].ActiveCount() < wsconns[j].ActiveCount()
	})

	for _, wsconn := range wsconns {
		if !wsconn.IsConnected() {
			err := Connect(wsconn, false)
			if err != nil {
				logger.Error.Println("ws, connect error", err)
				continue
			}
		}

		wsconn.AddActive(1)
		wsconn.SignalConnChan()
		return wsconn, nil
	}

	return nil, errors.New("all wsconns are not reachable")
}

func (ws *WSConn) ActiveCount() int64 {
	return atomic.LoadInt64(&ws.Active)
}

func (ws *WSConn) AddActive(delta int64) {
	atomic.AddInt64(&ws.Active, delta)
}

func (ws *WSConn) CanConnectNow() bool {
	ws.RWLock.RLock()
	defer ws.RWLock.RUnlock()
	return ws.CanConnect || ws.Connected
}

func (ws *WSConn) IsConnected() bool {
	ws.RWLock.RLock()
	defer ws.RWLock.RUnlock()
	return ws.Connected
}

func Connect(wsconn *WSConn, force bool) error {
	logger.Debug.Println(wsconn.Wid, "ws, try connect")

	wsconn.ConnectLock.Lock()
	defer func() {
		wsconn.ConnectLock.Unlock()
		logger.Debug.Println(wsconn.Wid, "ws, try connect done")
	}()

	if wsconn.IsConnected() {
		logger.Debug.Println(wsconn.Wid, "ws, connected by others")
		wsconn.SignalConnChan()
		return nil
	}
	if wsconn.Local.IsStopped() {
		return errors.New("local server is stopped")
	}

	wsconn.RWLock.RLock()
	canConnect := wsconn.CanConnect
	wsconn.RWLock.RUnlock()
	if !canConnect && !force {
		// let the blocked websocket puller try force connect
		wsconn.SignalConnChan()
		return errors.New("can not connect")
	}

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
	wsconn.WriteLock.Lock()
	wsconn.WSConn = conn
	wsconn.WriteLock.Unlock()

	wsconn.RWLock.Lock()
	logger.Debug.Println(wsconn.Wid, "ws, connected")
	wsconn.Connected = true
	wsconn.CanConnect = true
	wsconn.RWLock.Unlock()

	wsconn.SignalConnChan()
	return nil
}

func (ws *WSConn) SignalConnChan() {
	ws.RWLock.Lock()
	defer ws.RWLock.Unlock()
	if !isClosed(ws.ConnChan) {
		close(ws.ConnChan)
	}
}

func isClosed(ch <-chan interface{}) bool {
	select {
	case <-ch:
		return true
	default:
	}

	return false
}

// WebsocketPuller
// 1. pull message from websocket, put it to data channel
// 2. switch to a new transport connection when ttl is reached
func (ws *WSConn) WebsocketPuller() error {
	var switchTimer *time.Timer
	defer func() {
		stopTimer(switchTimer)
	}()

	logger.Debug.Println(ws.Wid, "ws, start")
	for {
		if ws.Local.IsStopped() {
			logger.Debug.Println(ws.Wid, "ws, local stopped")
			return nil
		}

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
			if isClosed(ws.ConnChan) {
				ws.ConnChan = make(chan interface{})
			}
			connChan := ws.ConnChan
			ws.RWLock.Unlock()
			logger.Debug.Println(ws.Wid, "ws, num of conns == 0, block on ConnChan")
			select {
			case <-connChan:
			case <-ws.Local.DoneChan():
				logger.Debug.Println(ws.Wid, "ws, stopped while idle")
				return nil
			}
			resetTimer(&switchTimer, time.Second*time.Duration(ws.TimeToLive))
		}

		// try connect if not connected
		if !ws.IsConnected() {
			logger.Debug.Println(ws.Wid, "ws, wait for reconnect", numOfConns)
			select {
			case <-time.After(time.Second * RECONNECT_INTERVAL):
			case <-ws.Local.DoneChan():
				logger.Debug.Println(ws.Wid, "ws, stopped before reconnect")
				return nil
			}
			err := Connect(ws, true)
			if err != nil {
				continue
			}
			resetTimer(&switchTimer, time.Second*time.Duration(ws.TimeToLive))
		}

		// switch conn when ttl is reached
		if switchTimer != nil {
			select {
			case <-switchTimer.C:
				switchTimer = nil
				// create new connection
				logger.Debug.Println(ws.Wid, "ws, switch start")
				wsconn := NewWSConn(ws.Url, ws.Wid, ws.Local)
				err := Connect(wsconn, false)
				if err != nil {
					// use old
					resetTimer(&switchTimer, time.Second*time.Duration(ws.TimeToLive))
					break
				}

				// send switch cmd (new wsconn)
				err = wsconn.WriteMessage(&common.Message{
					Wid: ws.Wid,
					Cmd: common.SWITCH,
				})
				if err != nil {
					// use old
					resetTimer(&switchTimer, time.Second*time.Duration(ws.TimeToLive))
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
				oldWSConn := ws.WSConn
				ws.WSConn = wsconn.WSConn
				if oldWSConn != nil {
					oldWSConn.Close()
				}
				ws.WriteLock.Unlock()

				// reset connection status
				ws.RWLock.Lock()
				ws.Connected = true
				ws.RWLock.Unlock()
				resetTimer(&switchTimer, time.Second*time.Duration(ws.TimeToLive))
			default:
			}
		}

		// read & put queue
		logger.Debug.Println(ws.Wid, "ws, wait read")
		msg, err := ws.ReadMessage()
		if err != nil {
			if ws.Local.IsStopped() {
				logger.Debug.Println(ws.Wid, "ws, stopped after read error")
				return nil
			}
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
	if ws.Local.IsStopped() {
		return errors.New("local server is stopped")
	}
	data, err := ws.Packer.Pack(msg)
	if err != nil {
		return err
	}
	ws.WriteLock.Lock()
	conn := ws.WSConn
	if conn == nil {
		ws.WriteLock.Unlock()
		ws.RWLock.Lock()
		ws.Connected = false
		ws.RWLock.Unlock()
		return errors.New("websocket is not connected")
	}
	err = conn.WriteMessage(websocket.BinaryMessage, data)
	ws.RWLock.Lock()
	if err != nil {
		ws.Connected = false
	}
	ws.RWLock.Unlock()
	ws.WriteLock.Unlock()
	return err
}

func resetTimer(timer **time.Timer, duration time.Duration) {
	stopTimer(*timer)
	*timer = time.NewTimer(duration)
}

func stopTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func (ws *WSConn) ReadMessage() (*common.Message, error) {
	for {
		mt, data, err := ws.WSConn.ReadMessage()
		if err != nil {
			ws.RWLock.Lock()
			ws.Connected = false
			ws.RWLock.Unlock()
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
