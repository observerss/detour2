package server

import (
	"detour/relay"
	"detour/schema"
	"log"

	"github.com/gorilla/websocket"
)

var password = "by160101"

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
		log.Printf("unpack error: %s", err)
		return
	}
	if msg.Data.CMD == schema.CONNECT {
		// do connect
		log.Println("connect:", msg.Data)

		// connect success, update tracker, make response
		h.Tracker.Add(msg.Pair, *msg.Data)

		// create puller goroutine (remote => local)
	} else if msg.Data.CMD == schema.DATA {
		// find conn by tracker

		// push data => remote
	} else {
		c.Close()
	}
}
