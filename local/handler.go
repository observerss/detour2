package local

import (
	"detour/relay"
	"errors"
	"io"
	"log"
	"net"
	"strings"

	"github.com/google/uuid"
)

// for compatibility with shadowsocks
const DOWNSTREAM_BUFSIZE = 16 * 1024
const UPSTREAM_BUFSIZE = 32 * 1024

type Handler struct {
	Pair     *relay.ConnPair
	Conn     net.Conn
	Client   *Client
	Quit     chan struct{}
	Stop     chan interface{}
	Protocol Protocol
	Closed   bool
}

type Protocol interface {
	Init() *Remote
	Bind(error) error
}

type Remote struct {
	Network string `json:"network,omitempty" comment:"e.g. tcp/udp"`
	Address string `json:"address,omitempty" comment:"e.g. example.com:443"`
}

func NewHandler(proto string, conn net.Conn, client *Client, quit chan struct{}) (*Handler, error) {
	handler := &Handler{Conn: conn, Client: client, Quit: quit}
	switch proto {
	case "shadowsocks":
		handler.Protocol = &ShadowProtocol{Conn: conn}
	case "socks5":
		handler.Protocol = &Socks5Protocol{Conn: conn}
	default:
		return nil, errors.New("proto not supported: " + proto)
	}
	connid, _ := uuid.NewRandom()
	log.Println("new handler: ", connid)
	handler.Pair = &relay.ConnPair{ClientId: CLIENTID, ConnId: connid}
	handler.Stop = make(chan interface{})
	return handler, nil
}

func (h *Handler) HandleConn() {
	log.Println("handle start.")
	remote := h.Protocol.Init()
	if remote == nil {
		h.Conn.Close()
		return
	}

	connByPair, err := h.Client.Connect(h.Pair, remote)
	log.Println("connect ok")

	err = h.Protocol.Bind(err)
	if err != nil {
		log.Println("connect error:", err)
		h.Conn.Close()
		connByPair.Close()
		return
	}

	log.Println("bind ok:", remote.Address)

	// spawn local => remote
	go h.Copy(h.Conn, connByPair, DOWNSTREAM_BUFSIZE, "local => remote")

	// remote => local
	h.Copy(connByPair, h.Conn, UPSTREAM_BUFSIZE, "remote => local")
}

func (h *Handler) Copy(src io.ReadCloser, dst io.WriteCloser, bufsize uint16, direction string) {
	buf := make([]byte, bufsize)

	defer func() {
		dst.Close()
		src.Close()
		log.Println(direction, "closed")
	}()

	for {
		select {
		case <-h.Quit:
			log.Println(direction, "quit")
			return
		case <-h.Stop:
			log.Println(direction, "stop:")
			return
		default:
		}

		nr, err := src.Read(buf)
		if err != nil && err != io.EOF {
			if !strings.Contains(err.Error(), "use of closed network connection") {
				log.Println(direction, "read error:", err)
			}
			break
		}
		nw, err := dst.Write(buf[0:nr])
		if err != nil {
			if !strings.Contains(err.Error(), "use of closed network connection") {
				log.Println(direction, "write error:", err)
			}
			break
		}
		log.Println(direction, ":", nw)
		if nw == 0 {
			break
		}
	}
}
