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
	connid, _ := uuid.NewUUID()
	handler.Pair = &relay.ConnPair{ClientId: CLIENTID, ConnId: connid}
	handler.Stop = make(chan interface{})
	return handler, nil
}

func (h *Handler) HandleConn() {
	remote := h.Protocol.Init()

	connByPair, err := h.Client.Connect(h.Pair, remote)

	err = h.Protocol.Bind(err)
	if err != nil {
		log.Println("connect error:", err)
		h.Conn.Close()
		connByPair.Close()
		return
	}

	log.Println("connect ok:", remote.Address)

	// spawn local => remote
	go h.Copy(h.Conn, connByPair, DOWNSTREAM_BUFSIZE, "local => remote")

	// remote => local
	h.Copy(connByPair, h.Conn, UPSTREAM_BUFSIZE, "remote => local")
}

func (h *Handler) Copy(src io.ReadCloser, dst io.WriteCloser, bufsize uint16, direction string) {
	buf := make([]byte, bufsize)
	read := make(chan int)
	waitread := make(chan interface{})
	go func() {
		for {
			nr, err := src.Read(buf)
			if err != nil && err != io.EOF {
				if !strings.Contains(err.Error(), "closed") {
					log.Println(direction, "read error:", err)
				}
				h.Stop <- nil
				src.Close()
				return
			}
			read <- nr
			if nr == 0 {
				src.Close()
				break
			}
			<-waitread
		}
	}()

	for {
		select {
		case <-h.Quit:
			log.Println(direction, "quit")
			return
		case <-h.Stop:
			log.Println(direction, "stop:")
			// time.Sleep(time.Second)
			dst.Close()
			return
		case nr := <-read:
			// log.Println(direction, "write ===> ", buf[0:100])
			nw, err := dst.Write(buf[0:nr])
			if err != nil {
				log.Println(direction, "write error:", err)
				return
			}
			log.Println(direction, ":", nw)

			if nw == 0 {
				h.Stop <- nil
				// time.Sleep(time.Second)
				dst.Close()
				return
			}
			waitread <- nil
		}
	}
}
