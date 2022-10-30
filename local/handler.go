package local

import (
	"detour/relay"
	"errors"
	"io"
	"log"
	"net"
	"time"

	"github.com/google/uuid"
)

// for compatibility with shadowsocks
const DOWNSTREAM_BUFSIZE = 16 * 1024
const UPSTREAM_BUFSIZE = 32 * 1024

type Handler struct {
	Pair       *relay.ConnPair
	Conn       net.Conn
	Client     *Client
	Quit       chan struct{}
	Stop       chan struct{}
	Negotiator Negotiator
}

type Negotiator interface {
	Negotiate() *Remote
}

type Remote struct {
	Network string `json:"network,omitempty" comment:"e.g. tcp/udp"`
	Address string `json:"address,omitempty" comment:"e.g. example.com:443"`
}

func NewHandler(proto string, conn net.Conn, client *Client, quit chan struct{}) (*Handler, error) {
	handler := &Handler{Conn: conn, Client: client, Quit: quit}
	switch proto {
	case "shadowsocks":
		handler.Negotiator = &ShadowNegotiator{Handler: handler}
	case "socks5":
		handler.Negotiator = &Socks5Negotiator{Handler: handler}
	default:
		return nil, errors.New("proto not supported: " + proto)
	}
	connid, _ := uuid.NewUUID()
	handler.Pair = &relay.ConnPair{ClientId: CLIENTID, ConnId: connid}
	handler.Stop = make(chan struct{})
	return handler, nil
}

func (h *Handler) HandleConn() {
	remote := h.Negotiator.Negotiate()

	err := h.Client.Connect(h.Pair, remote)
	if err != nil {
		log.Println("connect error:", h.Pair, err)
		h.Conn.Close()
		h.Client.Close(h.Pair)
		return
	}

	// spawn local => remote
	go h.Copy(h.Conn, h.Client.GetWriter(h.Pair), DOWNSTREAM_BUFSIZE, "local => remote")

	// remote => local
	h.Copy(h.Client.GetReader(h.Pair), h.Conn, UPSTREAM_BUFSIZE, "remote => local")
}

func (h *Handler) Copy(src io.Reader, dst io.Writer, bufsize uint16, direction string) {
	defer func() {
		// sleep before close, let linger data, if has, to go
		time.Sleep(time.Second)
		h.Client.Close(h.Pair)
		h.Conn.Close()
	}()

	written := 0
	buf := make([]byte, bufsize)

	for {
		select {
		case <-h.Quit:
			log.Println(h.Pair, direction, "quit")
			return
		case <-h.Stop:
			log.Println(h.Pair, direction, "stop:")
			return
		default:
		}

		nr, err := src.Read(buf)
		if err != nil {
			if err == io.EOF {
				dst.Write([]byte{})
				log.Println(h.Pair, direction, "read eof, total copied:", written)
			} else {
				log.Println(h.Pair, direction, "read error:", err, "total copied:", written)
			}
			close(h.Stop)
			return
		}

		nw, err := dst.Write(buf[0:nr])
		if err != nil {
			log.Println(h.Pair, direction, "write error:", err, "total copied", written)
			return
		}

		written += nw
	}
}
