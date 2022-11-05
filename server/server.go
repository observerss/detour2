package server

import (
	"context"
	"detour/common"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const BUFZIE = 16 * 1024

var upgrader = websocket.Upgrader{}

type Server struct {
	Address   string
	Conns     sync.Map                // Cid => Conn
	WSCounter map[*websocket.Conn]int // WSconn => num of NetConns
}

type Conn struct {
	Cid     string
	Network string
	Address string
	WSConn  *websocket.Conn
	NetConn net.Conn
	WSLock  *sync.Mutex
}

type Lock struct {
	WSLock sync.Mutex
}

type Handle struct {
	WSLock *sync.Mutex
	WSConn *websocket.Conn
	Msg    *common.Message
}

func NewServer(listen string) *Server {
	vals := strings.Split(listen, "://")
	server := &Server{
		Address: vals[1],
	}
	return server
}

func (s *Server) RunServer() {
	http.HandleFunc("/", s.HandleIndex)
	http.HandleFunc("/ws", s.HandleWebsocket)

	log.Printf("Listening at http://%s", s.Address)

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
			log.Printf("HTTP Server Shutdown Error: %v", err)
		}
		close(quit)
	}()

	// listen and block
	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("HTTP server ListenAndServe Error: %v", err)
	}

	<-quit

	log.Printf("Bye bye")
}

func (s *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
	log.Printf("HTTP %s %s%s\n", r.Method, r.Host, r.URL)

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
		log.Print("ws, upgrade error", err)
		return
	}
	log.Println("ws, upgraded")

	// wid, _ := common.GenerateRandomStringURLSafe(8)
	// s.Locks.Store(wid, &Lock{})
	lock := sync.Mutex{}

	defer func() {
		conn.Close()
	}()

	for {
		log.Println("ws, wait read")
		mt, data, err := conn.ReadMessage()
		if err != nil {
			log.Println("ws, ignore message type", mt)
			return
		}

		if mt != websocket.BinaryMessage {
			continue
		}

		msg, err := common.Unpack(data)
		if err != nil {
			log.Println("ws, unpack error", err)
			return
		}

		log.Println(msg.Cid, "ws, read", msg.Cmd, len(msg.Data))
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
			log.Println("ws, ", msg.Cmd, "not implemented")
		}

	}
}

func (s *Server) HandleConnect(handle *Handle) {
	msg := handle.Msg
	cid := msg.Cid
	conn := Conn{
		Cid:     cid,
		Network: msg.Network,
		Address: msg.Address,
		WSConn:  handle.WSConn,
		WSLock:  handle.WSLock,
	}

	log.Println(cid, "connect, open connection", msg.Network, msg.Address)

	dialer := net.Dialer{Timeout: time.Second * 3}

	remote, err := dialer.Dial(msg.Network, msg.Address)
	if err != nil {
		msg.Ok = false
		s.SendWebosket(&conn, msg)
		return
	}

	log.Println(cid, "connect, send ok")
	conn.NetConn = remote
	s.Conns.Store(cid, &conn)
	msg.Ok = true
	err = s.SendWebosket(&conn, msg)
	if err != nil {
		return
	}

	go s.RunLoop(&conn)
	log.Println(cid, "connect, done")
}

func (s *Server) RunLoop(conn *Conn) {
	log.Println(conn.Cid, "loop, start")
	defer func() {
		log.Println(conn.Cid, "loop, quit")
		conn.NetConn.Close()
		s.Conns.Delete(conn.Cid)

		// recalculate wscounter
		conn.WSLock.Lock()
		n, ok := s.WSCounter[conn.WSConn]
		if ok {
			n -= 1
			if n == 0 {
				delete(s.WSCounter, conn.WSConn)

				// if no clients on on current wsconn, close it
				log.Println("!!!! close websocket now")
				conn.WSConn.Close()
			} else {
				log.Println(conn.WSConn, "current num of NetConns", n)
				s.WSCounter[conn.WSConn] = n
			}
		}
		conn.WSLock.Unlock()
	}()

	// calculate wscounter
	conn.WSLock.Lock()
	n, ok := s.WSCounter[conn.WSConn]
	if !ok {
		n = 0
	}
	n += 1
	s.WSCounter[conn.WSConn] = n
	log.Println(conn.WSConn, "current num of NetConns", n)
	conn.WSLock.Unlock()

	buf := make([]byte, BUFZIE)
	for {
		log.Println(conn.Cid, "loop, wait read")

		conn.NetConn.SetReadDeadline(time.Now().Add(time.Second * 60))
		nr, err := conn.NetConn.Read(buf)
		if err != nil {
			log.Println(conn.Cid, "loop, read error", err)
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
		Network: msg.Network,
		Address: msg.Address,
	}

	var conn *Conn
	value, ok := s.Conns.Load(cid)
	if !ok {
		log.Println("data,", cid, "not found")
		log.Println(cid, "reconnect, open connection", msg.Network, msg.Address)
		dialer := net.Dialer{Timeout: time.Second * 3}
		remote, err := dialer.Dial(msg.Network, msg.Address)
		conn = &Conn{
			Cid:     cid,
			WSLock:  &sync.Mutex{},
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

	log.Println(cid, "data, send ===> website", len(msg.Data))
	_, err := conn.NetConn.Write(msg.Data)
	if err != nil {
		log.Println(cid, "data, write error", err)
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
	conn := handle.WSConn

	count := 0
	total := 0
	cid := msg.Cid
	s.Conns.Range(func(key, value any) bool {
		total += 1
		sconn := value.(*Conn)
		if key.(string) == cid && conn != sconn.WSConn {
			sconn.WSLock.Lock()
			sconn.WSConn = conn
			sconn.WSLock.Unlock()
			count += 1
		}
		return true
	})
	log.Println(cid, "switched", count, "of", total)
}

func (s *Server) SendWebosket(conn *Conn, msg *common.Message) error {
	conn.WSLock.Lock()
	defer conn.WSLock.Unlock()
	data, err := common.Pack(msg)
	if err != nil {
		return err
	}
	log.Println(msg.Cid, "send ===> websocket", msg.Cmd, len(msg.Data))
	return conn.WSConn.WriteMessage(websocket.BinaryMessage, data)
}
