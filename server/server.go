package server

import (
	"context"
	"detour/common"
	"errors"
	"io"
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

var upgrader = websocket.Upgrader{}

type Server struct {
	Address string
	Conns   sync.Map // CID => Conn
	Locks   sync.Map // Wid => WSConn Lock
}

type Conn struct {
	Cid    string
	WSConn *websocket.Conn
	Writer io.WriteCloser
}

type Handle struct {
	Wid  string
	Conn *websocket.Conn
	Msg  *common.Message
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

	wid, _ := common.GenerateRandomStringURLSafe(8)
	s.Locks.Store(wid, sync.Mutex{})

	defer func() {
		conn.Close()
		s.Locks.Delete(wid)
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
		handle := &Handle{Conn: conn, Msg: msg, Wid: wid}

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
	conn := handle.Conn
	wid := handle.Wid
	cid := msg.Cid
	log.Println(cid, "connect, open connection", msg.Network, msg.Address)

	dialer := net.Dialer{Timeout: time.Second * 3}

	remote, err := dialer.Dial(msg.Network, msg.Address)
	if err != nil {
		msg.Ok = false
		s.SendWebosket(wid, conn, msg)
	}

	// TODO
}

func (s *Server) HandleData(handle *Handle) {
}

func (s *Server) HandleClose(handle *Handle) {
}

func (s *Server) HandleSwitch(handle *Handle) {
	msg := handle.Msg
	conn := handle.Conn

	count := 0
	total := 0
	cid := msg.Cid
	s.Conns.Range(func(key, value any) bool {
		total += 1
		sconn := value.(Conn)
		if key.(string) == cid && conn != sconn.WSConn {
			sconn.WSConn = conn
			count += 1
		}
		return true
	})
	log.Println(cid, "switched", count, "of", total)
}

func (s *Server) SendWebosket(wid string, conn *websocket.Conn, msg *common.Message) error {
	value, ok := s.Locks.Load(wid)
	if !ok {
		return errors.New("lock not found")
	}
	lock := value.(sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	data, err := common.Pack(msg)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, data)
}
