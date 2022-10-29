package local

import (
	"log"

	"github.com/gorilla/websocket"
)

type Client struct {
	RemoteUrls  []string
	RemoteConns []*websocket.Conn
}

func NewClient(remoteUrls []string) *Client {
	return &Client{RemoteUrls: remoteUrls}
}

// KeepConnected connects and manages Remote WebSocket Servers
func (c *Client) KeepConnected() {
	// TODO: implement this
	for _, url := range c.RemoteUrls {
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			log.Println("connect to url error:", url, err)
		} else {
			c.RemoteConns = append(c.RemoteConns, conn)
		}
	}
	if len(c.RemoteConns) == 0 {
		log.Fatal("cannot connect to servers")
	}
}

func (c *Client) GetConn() *websocket.Conn {
	// TODO: implement this
	return nil
}
