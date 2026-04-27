package server

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

const RELAY_RECONNECT_INTERVAL = 1

type RelayClient struct {
	Url         string
	Wid         string
	Connected   bool
	WSConn      *websocket.Conn
	WriteLock   sync.Mutex
	RWLock      sync.RWMutex
	ConnectLock sync.Mutex
	Active      int64
	Packer      *common.Packer
	Server      *Server
}

func NewRelayClient(url string, wid string, server *Server) *RelayClient {
	return &RelayClient{
		Url:    strings.TrimSpace(url),
		Wid:    wid,
		Packer: server.Packer,
		Server: server,
	}
}

func (s *Server) HasNextRelay() bool {
	return len(s.RelayClients) > 0
}

func (s *Server) StartRelayClients() {
	s.RelayStartOnce.Do(func() {
		for _, relay := range s.RelayClients {
			go relay.WebsocketPuller()
		}
	})
}

func (s *Server) GetRelayClient() (*RelayClient, error) {
	s.StartRelayClients()
	relays := make([]*RelayClient, 0, len(s.RelayClients))
	for _, relay := range s.RelayClients {
		relays = append(relays, relay)
	}
	sort.SliceStable(relays, func(i, j int) bool {
		return relays[i].ActiveCount() < relays[j].ActiveCount()
	})
	for _, relay := range relays {
		if err := relay.Connect(); err != nil {
			logger.Error.Println(relay.Wid, "relay, connect error", err)
			continue
		}
		relay.AddActive(1)
		return relay, nil
	}
	return nil, errors.New("all relay remotes are not reachable")
}

func (relay *RelayClient) ActiveCount() int64 {
	return atomic.LoadInt64(&relay.Active)
}

func (relay *RelayClient) AddActive(delta int64) {
	atomic.AddInt64(&relay.Active, delta)
}

func (relay *RelayClient) IsConnected() bool {
	relay.RWLock.RLock()
	defer relay.RWLock.RUnlock()
	return relay.Connected
}

func (relay *RelayClient) SetConnected(connected bool) {
	relay.RWLock.Lock()
	relay.Connected = connected
	relay.RWLock.Unlock()
}

func (relay *RelayClient) Connect() error {
	relay.ConnectLock.Lock()
	defer relay.ConnectLock.Unlock()

	if relay.IsConnected() {
		return nil
	}

	dialer := websocket.Dialer{HandshakeTimeout: time.Second * DIAL_TIMEOUT}
	conn, _, err := dialer.Dial(relay.Url, nil)
	if err != nil {
		relay.SetConnected(false)
		return err
	}

	relay.WriteLock.Lock()
	if relay.WSConn != nil {
		relay.WSConn.Close()
	}
	relay.WSConn = conn
	relay.WriteLock.Unlock()
	relay.SetConnected(true)
	logger.Info.Println(relay.Wid, "relay, connected", relay.Url)
	return nil
}

func (relay *RelayClient) WriteMessage(msg *common.Message) error {
	if err := relay.Connect(); err != nil {
		return err
	}
	data, err := relay.Packer.Pack(msg)
	if err != nil {
		return err
	}

	relay.WriteLock.Lock()
	conn := relay.WSConn
	if conn == nil {
		relay.WriteLock.Unlock()
		relay.SetConnected(false)
		return errors.New("relay websocket is not connected")
	}
	err = conn.WriteMessage(websocket.BinaryMessage, data)
	relay.WriteLock.Unlock()
	if err != nil {
		relay.SetConnected(false)
	}
	return err
}

func (relay *RelayClient) ReadMessage() (*common.Message, error) {
	relay.WriteLock.Lock()
	conn := relay.WSConn
	relay.WriteLock.Unlock()
	if conn == nil {
		relay.SetConnected(false)
		return nil, errors.New("relay websocket is not connected")
	}

	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			relay.SetConnected(false)
			return nil, err
		}
		if mt != websocket.BinaryMessage {
			continue
		}
		msg, err := relay.Packer.Unpack(data)
		if err != nil {
			return nil, err
		}
		return msg, nil
	}
}

func (relay *RelayClient) WebsocketPuller() {
	for {
		if !relay.IsConnected() {
			if err := relay.Connect(); err != nil {
				time.Sleep(time.Second * RELAY_RECONNECT_INTERVAL)
				continue
			}
		}

		msg, err := relay.ReadMessage()
		if err != nil {
			logger.Error.Println(relay.Wid, "relay, read error", err)
			continue
		}
		relay.Server.HandleRelayResponse(relay, msg)
	}
}

func (s *Server) HandleRelayResponse(relay *RelayClient, msg *common.Message) {
	value, ok := s.Conns.Load(msg.Cid)
	if !ok {
		logger.Debug.Println(msg.Cid, "relay, handler has quit, tell next relay to close")
		msg.Cmd = common.CLOSE
		msg.Data = []byte{}
		msg.Wid = relay.Wid
		relay.WriteMessage(msg)
		return
	}

	conn := value.(*Conn)
	msg.Wid = conn.Wid
	if err := s.SendWebosket(conn, msg); err != nil {
		logger.Debug.Println(msg.Cid, "relay, send upstream error", err)
	}
	if msg.Cmd == common.CLOSE || (msg.Cmd == common.CONNECT && !msg.Ok) {
		conn.ReleaseRelay()
		s.Conns.Delete(msg.Cid)
	}
}

func (s *Server) HandleRelayConnect(handle *Handle) {
	msg := handle.Msg
	cid := msg.Cid
	conn := Conn{
		Cid:     cid,
		Wid:     msg.Wid,
		Network: msg.Network,
		Address: msg.Address,
		WSConn:  handle.WSConn,
		WSLock:  handle.WSLock,
	}

	logger.Debug.Println(cid, "relay, open next connection", msg.Network, msg.Address)
	relay, err := s.GetRelayClient()
	if err != nil {
		msg.Ok = false
		msg.Msg = err.Error()
		s.SendWebosket(&conn, msg)
		return
	}

	conn.Relay = relay
	s.Conns.Store(cid, &conn)
	forwarded := *msg
	forwarded.Wid = relay.Wid
	if err := relay.WriteMessage(&forwarded); err != nil {
		s.Conns.Delete(cid)
		conn.ReleaseRelay()
		msg.Ok = false
		msg.Msg = err.Error()
		s.SendWebosket(&conn, msg)
		return
	}
	logger.Debug.Println(cid, "relay, connect forwarded")
}

func (s *Server) HandleRelayData(handle *Handle) {
	msg := handle.Msg
	cid := msg.Cid
	cmsg := &common.Message{
		Cmd:     common.CLOSE,
		Cid:     cid,
		Wid:     msg.Wid,
		Network: msg.Network,
		Address: msg.Address,
	}

	value, ok := s.Conns.Load(cid)
	if !ok {
		logger.Debug.Println("relay data,", cid, "not found")
		s.SendWebosket(&Conn{Cid: cid, Wid: msg.Wid, Network: msg.Network, Address: msg.Address, WSConn: handle.WSConn, WSLock: handle.WSLock}, cmsg)
		return
	}

	conn := value.(*Conn)
	if conn.Relay == nil {
		logger.Debug.Println(cid, "relay data, next relay missing")
		s.Conns.Delete(cid)
		s.SendWebosket(conn, cmsg)
		return
	}

	forwarded := *msg
	forwarded.Wid = conn.Relay.Wid
	if err := conn.Relay.WriteMessage(&forwarded); err != nil {
		logger.Debug.Println(cid, "relay data, write error", err)
		conn.ReleaseRelay()
		s.Conns.Delete(cid)
		s.SendWebosket(conn, cmsg)
	}
}
