package server

import (
	"detour/relay"
	"log"
	"net"

	"github.com/gorilla/websocket"
)

var password = "by160101"

const DOWNSTREAM_BUFSIZE = 32 * 1024

var CLOSE = []byte("")

type Handler struct {
	Tracker *Tracker
	Server  *Server
}

func NewHandler(server *Server) *Handler {
	return &Handler{Tracker: NewTracker(), Server: server}
}

func (h *Handler) HandleRelay(data []byte, c *websocket.Conn) {
	msg, err := relay.Unpack(data, password)
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
	log.Println("connect:", msg.Data)

	conn, err := createConnection(msg.Data)
	if err != nil {
		writeMessage(c, newErrorMessage(msg, err))
		return
	}

	// make response
	err = writeMessage(c, newOKMessage(msg))
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
			(*conn.RemoteConn).Close()
		}
	}()

	// start pulling (remote => local)
	buf := make([]byte, DOWNSTREAM_BUFSIZE)
	log.Println("start pulling:", conn)
	for {
		select {
		case <-conn.Quit:
			log.Println("quit pulling:", conn)
			return
		default:
		}

		n, err := (*conn.RemoteConn).Read(buf)
		if err != nil {
			log.Println("pull error:", conn, err)
			return
		}

		log.Println("pull data:", conn, n)
		writeMessage(conn.LocalConn, newDataMessage(msg, buf))
	}
}

func (h *Handler) handleData(msg *relay.RelayMessage, c *websocket.Conn) {
	// find conn by tracker
	conn := h.Tracker.Find(msg.Pair)

	// conn is closed
	if conn == nil || conn.RemoteConn == nil {
		writeMessage(c, newDataMessage(msg, CLOSE))
		return
	}

	// push data => remote
	n, err := (*conn.RemoteConn).Write(msg.Data.Data)
	if err != nil {
		log.Println("push error:", conn, err)
		conn.Quit <- nil
	} else {
		log.Println("push data", conn, n)
	}
}

func createConnection(req *relay.RelayData) (*relay.ConnInfo, error) {
	// TODO: implement this
	net.Dial(req.Network, req.Address)
	return nil, nil
}

func writeMessage(c *websocket.Conn, msg *relay.RelayMessage) error {
	return c.WriteMessage(websocket.BinaryMessage, relay.Pack(msg, password))
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

func newDataMessage(msg *relay.RelayMessage, data []byte) *relay.RelayMessage {
	return &relay.RelayMessage{
		Pair: msg.Pair,
		Data: &relay.RelayData{CMD: relay.DATA, Data: data},
	}
}
