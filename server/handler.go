package server

import (
	"detour/relay"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const DOWNSTREAM_BUFSIZE = 32 * 1024
const CONNECT_TIMEOUT = 5
const READWRITE_TIMEOUT = 60

var lock = sync.Mutex{}

type Handler struct {
	Tracker *Tracker
	Server  *Server
}

func NewHandler(server *Server) *Handler {
	return &Handler{Tracker: NewTracker(), Server: server}
}

func (h *Handler) HandleRelay(data []byte, c *websocket.Conn) {
	msg, err := relay.Unpack(data, h.Server.Password)
	// log.Println("read:", msg.Data)
	if err != nil {
		log.Println("unpack error:", err)
		return
	}
	switch msg.Data.CMD {
	case relay.CONNECT:
		go h.handleConnect(msg, c)
	case relay.DATA:
		go h.handleData(msg, c)
	default:
		log.Println("cmd not supported:", msg.Data.CMD)
		c.Close()
	}
}

func (h *Handler) handleConnect(msg *relay.RelayMessage, c *websocket.Conn) {
	// do connect
	log.Println("connect:", msg.Data.Address)

	conn, err := createConnection(msg.Data, c)
	if err != nil {
		h.writeMessage(c, newErrorMessage(msg, err))
		return
	}

	// make response
	err = h.writeMessage(c, newOKMessage(msg))
	if err != nil {
		log.Println("write response error:", err)
		return
	}

	// connect success, update tracker
	h.Tracker.Upsert(msg.Pair, conn)

	// cleanups
	defer func() {
		h.Tracker.Remove(msg.Pair)
		if conn.RemoteConn != nil {
			conn.RemoteConn.Close()
		}
	}()

	// start pulling (remote => local)
	buf := make([]byte, DOWNSTREAM_BUFSIZE)
	log.Println("start pulling:", conn.Network)
	read := make(chan int)
	waitread := make(chan interface{})

	go func() {
		// a background goroutine for reading data
		for {
			conn.RemoteConn.SetReadDeadline(time.Now().Add(time.Second * READWRITE_TIMEOUT))
			nr, err := conn.RemoteConn.Read(buf)
			if err != nil && err != io.EOF {
				if strings.Contains(err.Error(), "closed") {
					return
				}
				if !strings.Contains(err.Error(), "timeout") {
					log.Println("pull remote error:", conn, err)
				}
				close(conn.Quit)
				return
			}
			read <- nr
			if nr == 0 {
				break
			}
			<-waitread
		}
	}()

	// a foreground for loop to process
	for {
		select {
		case <-conn.Quit:
			log.Println("quit pulling:", conn.Network)
			return
		case nr := <-read:
			log.Println("remote => local data:", nr)
			err = h.writeMessage(conn.LocalConn, newDataMessage(msg, buf[0:nr]))
			if err != nil {
				log.Println("remote => local error:", err)
				break
			}
			if nr == 0 {
				break
			}
			// keep alive
			h.Tracker.ImAlive(msg.Pair)
			waitread <- nil
		}
	}
}

func (h *Handler) handleData(msg *relay.RelayMessage, c *websocket.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("Recovered in handleData", r)
		}
	}()

	// find conn by tracker
	conn := h.Tracker.Find(msg.Pair)

	// conn is closed
	if conn == nil || conn.RemoteConn == nil {
		// tell the local to reconnect
		h.writeMessage(c, newReconnectMessage(msg))
		return
	}

	h.Tracker.ImAlive(msg.Pair)

	// update LocalConn if needed
	if conn.LocalConn != c {
		h.writeMessage(conn.LocalConn, newSwitchMessage(msg))
		conn.LocalConn = c
	}

	// push data => remote
	conn.RemoteConn.SetWriteDeadline(time.Now().Add(time.Second * READWRITE_TIMEOUT))
	n, err := conn.RemoteConn.Write(msg.Data.Data)
	if err != nil {
		log.Println("push error:", err)
		close(conn.Quit)
	} else {
		log.Println("local => remote data:", n)
		if n == 0 {
			close(conn.Quit)
		}
	}
}

func createConnection(req *relay.RelayData, c *websocket.Conn) (*relay.ConnInfo, error) {
	conn, err := net.DialTimeout(req.Network, req.Address, time.Second*CONNECT_TIMEOUT)
	if err != nil {
		return nil, err
	}
	return &relay.ConnInfo{
		Network:    req.Network,
		Address:    req.Address,
		Activity:   time.Now().UnixMilli(),
		LocalConn:  c,
		RemoteConn: conn,
		Quit:       make(chan interface{}),
	}, nil
}

func (h *Handler) writeMessage(c *websocket.Conn, msg *relay.RelayMessage) error {
	// log.Println("write:", msg.Data)
	lock.Lock()
	defer lock.Unlock()
	return c.WriteMessage(websocket.BinaryMessage, relay.Pack(msg, h.Server.Password))
}

func newErrorMessage(msg *relay.RelayMessage, err error) *relay.RelayMessage {
	return &relay.RelayMessage{
		Pair: msg.Pair,
		Data: &relay.RelayData{CMD: relay.CONNECT, OK: false, MSG: err.Error()},
	}
}

func newOKMessage(msg *relay.RelayMessage) *relay.RelayMessage {
	return &relay.RelayMessage{
		Pair: msg.Pair,
		Data: &relay.RelayData{CMD: relay.CONNECT, OK: true},
	}
}

func newSwitchMessage(msg *relay.RelayMessage) *relay.RelayMessage {
	return &relay.RelayMessage{
		Pair: msg.Pair,
		Data: &relay.RelayData{CMD: relay.SWITCH, OK: true},
	}
}

func newReconnectMessage(msg *relay.RelayMessage) *relay.RelayMessage {
	return &relay.RelayMessage{
		Pair: msg.Pair,
		Data: &relay.RelayData{CMD: relay.RECONNECT, OK: true},
	}
}

func newDataMessage(msg *relay.RelayMessage, data []byte) *relay.RelayMessage {
	return &relay.RelayMessage{
		Pair: msg.Pair,
		Data: &relay.RelayData{CMD: relay.DATA, Data: data},
	}
}
