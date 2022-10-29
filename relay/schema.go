package relay

import (
	"net"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Command int

const (
	CONNECT Command = iota
	DATA
	SWITCH
)

type RelayData struct {
	CMD     Command `json:"cmd,omitempty"`
	OK      bool    `json:"ok,omitempty"`
	MSG     string  `json:"msg,omitempty"`
	Network string  `json:"network,omitempty" comment:"e.g. tcp/udp"`
	Address string  `json:"address,omitempty" comment:"e.g. example.com:443"`
	Data    []byte  `json:"data,omitempty"`
}

type ClientInfo struct {
	Conns    map[uuid.UUID]*ConnInfo `json:"conns,omitempty" comment:"connid => conninfo"`
	Activity int64                   `json:"-"`
}

type ConnInfo struct {
	Network    string           `json:"network,omitempty" comment:"e.g. tcp"`
	Address    string           `json:"address,omitempty" comment:"e.g. example.com:443"`
	Activity   int64            `json:"-"`
	RemoteConn *net.Conn        `json:"-"`
	LocalConn  *websocket.Conn  `json:"-"`
	Quit       chan interface{} `json:"-"`
}

type ConnPair struct {
	ClientId uuid.UUID
	ConnId   uuid.UUID
}
