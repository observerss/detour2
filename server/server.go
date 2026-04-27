package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/observerss/detour2/common"
	"github.com/observerss/detour2/logger"

	_ "net/http/pprof"

	"github.com/gorilla/websocket"
)

const (
	BUFFER_SIZE  = 64 * 1024
	READ_TIMEOUT = 60
	DIAL_TIMEOUT = 3
)

var upgrader = websocket.Upgrader{}

type Server struct {
	Address        string
	Packer         *common.Packer
	Conns          sync.Map       // Cid => Conn
	WSCounter      map[string]int // Wid => num of NetConns
	WSCounterLock  sync.Mutex
	RelayClients   map[string]*RelayClient
	RelayStartOnce sync.Once
	DNSServers     []string
	DNSCounter     uint64
	Metrics        *common.RuntimeMetrics
	MetricsListen  string
}

type Conn struct {
	Wid         string // wsid
	Cid         string // connid
	Network     string
	Address     string
	WSConn      *websocket.Conn
	NetConn     net.Conn
	WSLock      *sync.Mutex
	WSWriter    *common.FairMessageWriter
	TransportMu sync.RWMutex
	Relay       *RelayClient
	ReleaseOnce sync.Once
}

func (c *Conn) Transport() (*websocket.Conn, *common.FairMessageWriter) {
	c.TransportMu.RLock()
	defer c.TransportMu.RUnlock()
	return c.WSConn, c.WSWriter
}

func (c *Conn) SetTransport(wsconn *websocket.Conn, wslock *sync.Mutex, writer *common.FairMessageWriter) *common.FairMessageWriter {
	c.TransportMu.Lock()
	defer c.TransportMu.Unlock()
	oldWriter := c.WSWriter
	c.WSConn = wsconn
	c.WSLock = wslock
	c.WSWriter = writer
	return oldWriter
}

func (c *Conn) ReleaseRelay() {
	c.ReleaseOnce.Do(func() {
		if c.Relay != nil {
			c.Relay.AddActive(-1)
		}
	})
}

type Handle struct {
	WSLock   *sync.Mutex
	WSConn   *websocket.Conn
	Msg      *common.Message
	WSWriter *common.FairMessageWriter
}

func NewServer(sconf *common.ServerConfig) *Server {
	vals := strings.Split(sconf.Listen, "://")
	server := &Server{
		Address:       vals[1],
		Packer:        &common.Packer{Password: sconf.Password},
		WSCounter:     make(map[string]int),
		RelayClients:  make(map[string]*RelayClient),
		DNSServers:    ParseDNSServers(sconf.DNSServers),
		Metrics:       common.NewRuntimeMetrics(),
		MetricsListen: strings.TrimSpace(sconf.MetricsListen),
	}
	relayPoolSize := sconf.RelayPoolSize
	if relayPoolSize < 1 {
		relayPoolSize = 1
	}
	for _, url := range strings.Split(sconf.Remotes, ",") {
		url = strings.TrimSpace(url)
		if url == "" {
			continue
		}
		for i := 0; i < relayPoolSize; i++ {
			wid, _ := common.GenerateRandomStringURLSafe(3)
			server.RelayClients[fmt.Sprintf("%s#%d", url, i)] = NewRelayClient(url, wid, server)
		}
	}
	return server
}

func (s *Server) RunServer() {
	s.StartRelayClients()
	s.StartMetricsServer()
	http.HandleFunc("/ws", s.HandleWebsocket)
	http.HandleFunc("/", s.HandleIndex)

	logger.Info.Printf("Listening at http://%s", s.Address)

	httpServer := &http.Server{
		Addr: s.Address,
	}

	// graceful shutdown
	quit := make(chan struct{})
	go func() {
		sigch := make(chan os.Signal, 1)
		signal.Notify(sigch, syscall.SIGINT, syscall.SIGTERM)
		<-sigch
		if err := httpServer.Shutdown(context.Background()); err != nil {
			logger.Info.Printf("HTTP Server Shutdown Error: %v", err)
		}
		close(quit)
	}()

	// listen and block
	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		logger.Error.Fatalf("HTTP server ListenAndServe Error: %v", err)
	}

	<-quit

	logger.Debug.Printf("Bye bye")
}

func (s *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
	logger.Debug.Printf("HTTP %s %s%s\n", r.Method, r.Host, r.URL)

	if r.URL.Path != "/" {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(time.Now().Local().Format(time.RFC3339)))
}

func (s *Server) HandleWebsocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Debug.Println("ws, upgrade error", err)
		return
	}
	logger.Info.Println("ws, connected")
	if s.Metrics != nil {
		s.Metrics.WebSocketConnectsTotal.Inc()
		s.Metrics.WebSocketActive.Inc()
	}

	// wid, _ := common.GenerateRandomStringURLSafe(8)
	// s.Locks.Store(wid, &Lock{})
	lock := sync.Mutex{}
	writer := s.NewWebsocketWriter(conn)

	defer func() {
		s.CloseWebsocketConns(conn, writer)
		if s.Metrics != nil {
			s.Metrics.WebSocketActive.Dec()
		}
		writer.Close()
		conn.Close()
	}()

	for {
		logger.Debug.Println("ws, wait read")
		// conn.SetReadDeadline(time.Now().Add(time.Second * READ_TIMEOUT))
		mt, data, err := conn.ReadMessage()
		if err != nil {
			if s.Metrics != nil {
				s.Metrics.WebSocketReadErrors.Inc()
			}
			logger.Debug.Println("ws, read error", err)
			return
		}

		if mt != websocket.BinaryMessage {
			continue
		}

		msg, err := s.Packer.Unpack(data)
		if err != nil {
			logger.Debug.Println("ws, unpack error", err)
			return
		}
		if s.Metrics != nil {
			s.Metrics.MessagesInTotal.Inc()
			s.Metrics.PayloadBytesInTotal.Add(int64(len(msg.Data)))
		}

		logger.Debug.Println(msg.Cid, "ws, read", msg.Cmd, len(msg.Data))
		handle := &Handle{
			WSConn:   conn,
			Msg:      msg,
			WSLock:   &lock,
			WSWriter: writer,
		}

		switch msg.Cmd {
		case common.CONNECT:
			s.HandleConnect(handle)
		case common.DATA:
			s.HandleData(handle)
		case common.CLOSE:
			s.HandleClose(handle)
		case common.SWITCH:
			s.HandleSwitch(handle)
		default:
			logger.Warn.Println("ws, ", msg.Cmd, "not implemented")
		}

	}
}

func (s *Server) CloseWebsocketConns(wsconn *websocket.Conn, writer *common.FairMessageWriter) {
	s.Conns.Range(func(key, value any) bool {
		conn := value.(*Conn)
		connWS, connWriter := conn.Transport()
		if connWS != wsconn && connWriter != writer {
			return true
		}

		if conn.Relay != nil {
			msg := &common.Message{
				Cmd:     common.CLOSE,
				Cid:     conn.Cid,
				Wid:     conn.Relay.Wid,
				Network: conn.Network,
				Address: conn.Address,
			}
			if err := conn.Relay.WriteMessage(msg); err != nil {
				logger.Debug.Println(conn.Cid, "ws close, relay close write error", err)
			}
			conn.ReleaseRelay()
		} else if conn.NetConn != nil {
			conn.NetConn.Close()
		}
		s.Conns.Delete(key)
		return true
	})
}

func (s *Server) CloseRelayConns(relay *RelayClient) {
	s.Conns.Range(func(key, value any) bool {
		conn := value.(*Conn)
		if conn.Relay != relay {
			return true
		}

		msg := &common.Message{
			Cmd:     common.CLOSE,
			Cid:     conn.Cid,
			Wid:     conn.Wid,
			Network: conn.Network,
			Address: conn.Address,
		}
		if err := s.SendWebosket(conn, msg); err != nil {
			logger.Debug.Println(conn.Cid, "relay close, upstream close write error", err)
		}
		conn.ReleaseRelay()
		s.Conns.Delete(key)
		return true
	})
}

func (s *Server) HandleConnect(handle *Handle) {
	if s.HasNextRelay() {
		s.HandleRelayConnect(handle)
		return
	}

	msg := handle.Msg
	cid := msg.Cid
	conn := Conn{
		Cid:      cid,
		Wid:      msg.Wid,
		Network:  msg.Network,
		Address:  msg.Address,
		WSConn:   handle.WSConn,
		WSLock:   handle.WSLock,
		WSWriter: handle.WSWriter,
	}

	logger.Debug.Println(cid, "connect, open connection", msg.Network, msg.Address)
	if s.Metrics != nil {
		s.Metrics.ConnectAttemptsTotal.Inc()
	}

	dialer := s.Dialer()

	remote, err := dialer.Dial(msg.Network, msg.Address)
	if err != nil {
		if s.Metrics != nil {
			s.Metrics.ConnectFailuresTotal.Inc()
		}
		msg.Ok = false
		msg.Msg = err.Error()
		logger.Error.Println(cid, "connect, failed", err)
		s.SendWebosket(&conn, msg)
		return
	}

	logger.Debug.Println(cid, "connect, send ok")
	conn.NetConn = remote
	s.Conns.Store(cid, &conn)
	msg.Ok = true
	err = s.SendWebosket(&conn, msg)
	if err != nil {
		return
	}

	go s.RunLoop(&conn)
	logger.Debug.Println(cid, "connect, done")
}

func (s *Server) incrementWSCounter(wid string) int {
	s.WSCounterLock.Lock()
	defer s.WSCounterLock.Unlock()
	if s.WSCounter == nil {
		s.WSCounter = make(map[string]int)
	}
	n := s.WSCounter[wid] + 1
	s.WSCounter[wid] = n
	return n
}

func (s *Server) decrementWSCounter(wid string) int {
	s.WSCounterLock.Lock()
	defer s.WSCounterLock.Unlock()
	n, ok := s.WSCounter[wid]
	if !ok {
		return 0
	}
	n -= 1
	if n <= 0 {
		delete(s.WSCounter, wid)
		return 0
	}
	s.WSCounter[wid] = n
	return n
}

func (s *Server) RunLoop(conn *Conn) {
	// TODO: debug this, the loop is broken
	logger.Debug.Println(conn.Cid, "loop, start")
	defer func() {
		logger.Debug.Println(conn.Cid, "loop, quit")
		conn.NetConn.Close()
		s.Conns.Delete(conn.Cid)

		// recalculate wscounter
		n := s.decrementWSCounter(conn.Wid)
		logger.Debug.Println(conn.Wid, "current num of NetConns", n)
	}()

	// calculate wscounter
	s.incrementWSCounter(conn.Wid)

	buf := make([]byte, BUFFER_SIZE)
	for {
		logger.Debug.Println(conn.Cid, "loop, wait read")

		conn.NetConn.SetReadDeadline(time.Now().Add(time.Second * READ_TIMEOUT))
		nr, err := conn.NetConn.Read(buf)
		if err != nil {
			logger.Debug.Println(conn.Cid, "loop, read error", err)
			if _, ok := s.Conns.Load(conn.Cid); ok {
				s.SendWebosket(conn, &common.Message{
					Cmd:     common.CLOSE,
					Cid:     conn.Cid,
					Wid:     conn.Wid,
					Network: conn.Network,
					Address: conn.Address,
				})
			}
			return
		}

		cmd := common.DATA
		if nr == 0 {
			cmd = common.CLOSE
		}
		data := append([]byte{}, buf[:nr]...)
		msg := &common.Message{
			Cmd:     cmd,
			Cid:     conn.Cid,
			Wid:     conn.Wid,
			Data:    data,
			Network: conn.Network,
			Address: conn.Address,
		}
		// DO NOT return on error here, the wsconn will be switched to recover
		s.SendWebosket(conn, msg)

		if cmd == common.CLOSE {
			return
		}
	}
}

func (s *Server) HandleData(handle *Handle) {
	if s.HasNextRelay() {
		s.HandleRelayData(handle)
		return
	}

	msg := handle.Msg
	cid := msg.Cid
	cmsg := &common.Message{
		Cmd:     common.CLOSE,
		Cid:     cid,
		Wid:     msg.Wid,
		Network: msg.Network,
		Address: msg.Address,
	}

	var conn *Conn
	value, ok := s.Conns.Load(cid)
	if !ok {
		logger.Debug.Println("data,", cid, "not found")
		logger.Debug.Println(cid, "reconnect, open connection", msg.Network, msg.Address)
		if s.Metrics != nil {
			s.Metrics.ConnectAttemptsTotal.Inc()
		}
		dialer := s.Dialer()
		remote, err := dialer.Dial(msg.Network, msg.Address)
		conn = &Conn{
			Cid:      cid,
			Wid:      msg.Wid,
			WSLock:   handle.WSLock,
			WSConn:   handle.WSConn,
			WSWriter: handle.WSWriter,
			NetConn:  remote,
			Network:  msg.Network,
			Address:  msg.Address,
		}
		if err != nil {
			if s.Metrics != nil {
				s.Metrics.ConnectFailuresTotal.Inc()
			}
			logger.Error.Println(cid, "reconnect, failed", err)
			s.SendWebosket(conn, cmsg)
			return
		}
		s.Conns.Store(cid, conn)
		go s.RunLoop(conn)
	} else {
		conn = value.(*Conn)
	}

	logger.Debug.Println(cid, "data, send ===> website", len(msg.Data))
	_, err := conn.NetConn.Write(msg.Data)
	if err != nil {
		logger.Debug.Println(cid, "data, write error", err)
		conn.NetConn.Close()
		s.Conns.Delete(cid)
		s.SendWebosket(conn, cmsg)
	}
}

func (s *Server) HandleClose(handle *Handle) {
	value, ok := s.Conns.Load(handle.Msg.Cid)
	if ok {
		conn := value.(*Conn)
		if conn.Relay != nil {
			msg := *handle.Msg
			msg.Wid = conn.Relay.Wid
			conn.Relay.WriteMessage(&msg)
			conn.ReleaseRelay()
			s.Conns.Delete(handle.Msg.Cid)
			return
		}
		conn.NetConn.Close()
		s.Conns.Delete(handle.Msg.Cid)
	}
}

func (s *Server) HandleSwitch(handle *Handle) {
	msg := handle.Msg
	newconn := handle.WSConn
	newlock := handle.WSLock
	newwriter := handle.WSWriter

	count := 0
	total := 0
	wid := msg.Wid
	oldWriters := map[*common.FairMessageWriter]bool{}
	s.Conns.Range(func(key, value any) bool {
		total += 1
		sconn := value.(*Conn)
		oldconn, _ := sconn.Transport()
		if sconn.Wid == msg.Wid && newconn != oldconn {
			oldWriter := sconn.SetTransport(newconn, newlock, newwriter)
			if oldWriter != nil && oldWriter != newwriter {
				oldWriters[oldWriter] = true
			}
			count += 1
		}
		return true
	})
	for writer := range oldWriters {
		writer.Close()
	}
	logger.Info.Println(wid, "switched", count, "of", total)
}

func (s *Server) SendWebosket(conn *Conn, msg *common.Message) error {
	_, writer := conn.Transport()
	logger.Debug.Println(msg.Cid, "send ===> websocket", msg.Cmd, len(msg.Data))
	return s.writeWebsocket(writer, msg)
}
