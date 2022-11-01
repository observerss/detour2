package server

import (
	"detour/relay"
	"io"
	"log"
	"net"
	"time"
)

const DOWNSTREAM_BUFSIZE = 32 * 1024
const CONNECT_TIMEOUT = 3
const READWRITE_TIMEOUT = 60

type Handler struct {
	Tracker *Tracker
	Server  *Server
}

func NewHandler(server *Server) *Handler {
	return &Handler{Tracker: NewTracker(), Server: server}
}

func (h *Handler) HandleRelay(data []byte, writer chan *relay.RelayMessage) {
	msg, err := relay.Unpack(data, h.Server.Password)
	if err != nil {
		log.Println("unpack error:", err)
		return
	}
	switch msg.Data.CMD {
	case relay.CONNECT:
		go h.handleConnect(msg, writer)
	case relay.DATA:
		go h.handleData(msg, writer)
	default:
		log.Println("cmd not supported:", msg.Data.CMD)
		writer <- nil
	}
}

func (h *Handler) handleConnect(msg *relay.RelayMessage, writer chan *relay.RelayMessage) {
	// do connect
	log.Println("connect:", msg.Data.Address)

	conn, err := createConnection(msg.Data, writer)
	if err != nil {
		log.Println("connect error:", err)
		writer <- newErrorMessage(msg, err)
		writer <- nil
		return
	}

	// make response
	writer <- newOKMessage(msg)

	// connect success, update tracker
	h.Tracker.Upsert(msg.Pair, conn)

	defer func() {
		log.Println("stopped pulling:", conn.Address)
		// h.Tracker.Remove(msg.Pair)
		if conn.RemoteConn != nil {
			conn.RemoteConn.Close()
		}
	}()

	// start pulling (remote => local)
	buf := make([]byte, DOWNSTREAM_BUFSIZE)
	log.Println("start pulling:", conn.Address)

	// a foreground for loop to process
	for {
		conn.RemoteConn.SetReadDeadline(time.Now().Add(time.Second * READWRITE_TIMEOUT))
		nr, err := conn.RemoteConn.Read(buf)
		if err != nil && err != io.EOF {
			log.Println("pull remote error:", err)
			break
		}

		writer <- newDataMessage(msg, buf[0:nr])
		log.Println("remote => local data:", nr)
		if nr == 0 {
			log.Println("return on 0")
			break
		}
		// keep alive
		// h.Tracker.ImAlive(msg.Pair)
	}
}

func (h *Handler) handleData(msg *relay.RelayMessage, writer chan *relay.RelayMessage) {
	// find conn by tracker
	conn := h.Tracker.Find(msg.Pair)

	// conn is closed
	if conn == nil || conn.RemoteConn == nil {
		// tell the local to reconnect
		writer <- newReconnectMessage(msg)
		return
	}

	h.Tracker.ImAlive(msg.Pair)

	// push data => remote
	conn.RemoteConn.SetWriteDeadline(time.Now().Add(time.Second * READWRITE_TIMEOUT))
	n, err := conn.RemoteConn.Write(msg.Data.Data)
	if err != nil {
		log.Println("write error:", err)
		conn.RemoteConn.Close()
		return
	}

	log.Println("local => remote data:", n)
	if n == 0 {
		conn.RemoteConn.Close()
	}
}

func createConnection(req *relay.RelayData, writer chan *relay.RelayMessage) (*relay.ConnInfo, error) {
	conn, err := net.DialTimeout(req.Network, req.Address, time.Second*CONNECT_TIMEOUT)
	if err != nil {
		return nil, err
	}
	return &relay.ConnInfo{
		Network:    req.Network,
		Address:    req.Address,
		Activity:   time.Now().UnixMilli(),
		RemoteConn: conn,
		Writer:     writer,
	}, nil
}

// func (h *Handler) writeMessage(c *websocket.Conn, msg *relay.RelayMessage) error {
// 	// log.Println("write:", msg.Data)
// 	lock.Lock()
// 	defer lock.Unlock()
// 	return c.WriteMessage(websocket.BinaryMessage, relay.Pack(msg, h.Server.Password))
// }

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

// func newSwitchMessage(msg *relay.RelayMessage) *relay.RelayMessage {
// 	return &relay.RelayMessage{
// 		Pair: msg.Pair,
// 		Data: &relay.RelayData{CMD: relay.SWITCH, OK: true},
// 	}
// }

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
