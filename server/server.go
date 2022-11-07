package server

import (
	"context"
	"detour/common"
	"detour/logger"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "net/http/pprof"

	"github.com/gorilla/websocket"
)

const (
	BUFFER_SIZE  = 16 * 1024
	READ_TIMEOUT = 60
	DIAL_TIMEOUT = 3
)

var upgrader = websocket.Upgrader{}

type Server struct {
	Address   string
	Packer    *common.Packer
	Conns     sync.Map       // Cid => Conn
	WSCounter map[string]int // Wid => num of NetConns
}

type Conn struct {
	Wid     string // wsid
	Cid     string // connid
	Network string
	Address string
	WSConn  *websocket.Conn
	NetConn net.Conn
	WSLock  *sync.Mutex
}

type Handle struct {
	WSLock *sync.Mutex
	WSConn *websocket.Conn
	Msg    *common.Message
}

func NewServer(sconf *common.ServerConfig) *Server {
	vals := strings.Split(sconf.Listen, "://")
	server := &Server{
		Address:   vals[1],
		Packer:    &common.Packer{Password: sconf.Password},
		WSCounter: make(map[string]int),
	}
	return server
}

func (s *Server) RunServer() {
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

	// wid, _ := common.GenerateRandomStringURLSafe(8)
	// s.Locks.Store(wid, &Lock{})
	lock := sync.Mutex{}

	defer func() {
		conn.Close()
	}()

	for {
		logger.Debug.Println("ws, wait read")
		// conn.SetReadDeadline(time.Now().Add(time.Second * READ_TIMEOUT))
		mt, data, err := conn.ReadMessage()
		if err != nil {
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

		logger.Debug.Println(msg.Cid, "ws, read", msg.Cmd, len(msg.Data))
		handle := &Handle{
			WSConn: conn,
			Msg:    msg,
			WSLock: &lock,
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

func (s *Server) HandleConnect(handle *Handle) {
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

	logger.Debug.Println(cid, "connect, open connection", msg.Network, msg.Address)

	dialer := net.Dialer{Timeout: time.Second * DIAL_TIMEOUT}

	remote, err := dialer.Dial(msg.Network, msg.Address)
	if err != nil {
		msg.Ok = false
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

func (s *Server) RunLoop(conn *Conn) {
	// TODO: debug this, the loop is broken
	logger.Info.Println(conn.Cid, "loop, start")
	defer func() {
		logger.Info.Println(conn.Cid, "loop, quit")
		conn.NetConn.Close()
		s.Conns.Delete(conn.Cid)

		// recalculate wscounter
		conn.WSLock.Lock()
		n, ok := s.WSCounter[conn.Wid]
		if ok {
			n -= 1
			if n == 0 {
				delete(s.WSCounter, conn.Wid)

				// if no clients on on current wsconn, close it
				logger.Info.Println(conn.Wid, "ws, closed")
				conn.WSConn.Close()
			} else {
				logger.Debug.Println(conn.Wid, "current num of NetConns", n)
				s.WSCounter[conn.Wid] = n
			}
		}
		conn.WSLock.Unlock()
	}()

	// calculate wscounter
	conn.WSLock.Lock()
	n, ok := s.WSCounter[conn.Wid]
	if !ok {
		n = 0
	}
	n += 1
	s.WSCounter[conn.Wid] = n
	conn.WSLock.Unlock()

	buf := make([]byte, BUFFER_SIZE)
	for {
		logger.Debug.Println(conn.Cid, "loop, wait read")

		conn.NetConn.SetReadDeadline(time.Now().Add(time.Second * READ_TIMEOUT))
		nr, err := conn.NetConn.Read(buf)
		if err != nil {
			logger.Debug.Println(conn.Cid, "loop, read error", err)
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
		dialer := net.Dialer{Timeout: time.Second * DIAL_TIMEOUT}
		remote, err := dialer.Dial(msg.Network, msg.Address)
		conn = &Conn{
			Cid:     cid,
			Wid:     msg.Wid,
			WSLock:  handle.WSLock,
			WSConn:  handle.WSConn,
			NetConn: remote,
			Network: msg.Network,
			Address: msg.Address,
		}
		s.Conns.Store(cid, conn)
		if err != nil {
			s.SendWebosket(conn, cmsg)
			return
		}
		go s.RunLoop(conn)
	} else {
		conn = value.(*Conn)
	}

	logger.Debug.Println(cid, "data, send ===> website", len(msg.Data))
	_, err := conn.NetConn.Write(msg.Data)
	if err != nil {
		logger.Debug.Println(cid, "data, write error", err)
		s.Conns.Delete(cid)
		s.SendWebosket(conn, cmsg)
	}
}

func (s *Server) HandleClose(handle *Handle) {
	value, ok := s.Conns.Load(handle.Msg.Cid)
	if ok {
		value.(*Conn).NetConn.Close()
	}
}

func (s *Server) HandleSwitch(handle *Handle) {
	msg := handle.Msg
	newconn := handle.WSConn
	newlock := handle.WSLock

	count := 0
	total := 0
	wid := msg.Wid
	s.Conns.Range(func(key, value any) bool {
		total += 1
		sconn := value.(*Conn)
		if sconn.Wid == msg.Wid && newconn != sconn.WSConn {
			oldlock := sconn.WSLock
			oldlock.Lock()
			sconn.WSConn = newconn
			sconn.WSLock = newlock
			oldlock.Unlock()
			count += 1
		}
		return true
	})
	logger.Info.Println(wid, "switched", count, "of", total)
}

func (s *Server) SendWebosket(conn *Conn, msg *common.Message) error {
	conn.WSLock.Lock()
	defer conn.WSLock.Unlock()
	data, err := s.Packer.Pack(msg)
	if err != nil {
		return err
	}
	logger.Debug.Println(msg.Cid, "send ===> websocket", msg.Cmd, len(msg.Data))
	return conn.WSConn.WriteMessage(websocket.BinaryMessage, data)
}
