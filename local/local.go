package local

import (
	"detour/common"
	"log"
	"net"
	"strings"
	"sync"
)

const BUFSIZE = 16 * 1024

type Local struct {
	Network string
	Address string
	Packer  *common.Packer
	Proto   Proto
	WSConns map[string]*WSConn // url => WSConn
	Conns   sync.Map           // Cid => Conn
}
type Conn struct {
	Cid     string
	Wid     string
	Quit    chan interface{}
	MsgChan chan *common.Message
	NetConn net.Conn
	WSConn  *WSConn
}

func NewLocal(lconf *common.LocalConfig) *Local {
	vals := strings.Split(lconf.Listen, "://")
	network := vals[0]
	address := vals[1]
	urls := strings.Split(lconf.Remotes, ",")
	local := &Local{
		Network: network,
		Address: address,
		Packer:  &common.Packer{Password: lconf.Password},
	}
	switch lconf.Proto {
	case PROTO_SOCKS5:
		local.Proto = &Socks5Proto{}
	default:
		log.Fatal("proto", lconf.Proto, "not supported")
	}
	for _, url := range urls {
		wid, _ := common.GenerateRandomStringURLSafe(3)
		local.WSConns[url] = NewWSConn(url, wid, local)
	}
	return local
}

func (l *Local) RunLocal() {
	// websocket.Dialer{HandshakeTimeout: time.Second * 3}
	listen, err := net.Listen(l.Network, l.Address)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Listening on " + l.Network + "://" + l.Address)

	// background wsconn puller & keeper
	for _, wsconn := range l.WSConns {
		go wsconn.WebsocketPuller()
	}

	for {
		conn, err := listen.Accept()
		if err != nil {
			log.Println("run, accept error", err)
			continue
		}

		go l.HandleConn(conn)
	}
}

func (l *Local) HandleConn(netconn net.Conn) {
	cid, _ := common.GenerateRandomStringURLSafe(6)
	handleOk := false
	defer func() {
		if !handleOk {
			log.Println(cid, "handle, close conn")
			netconn.Close()
			l.Conns.Delete(cid)
		}
	}()

	log.Println(cid, "handle, init")
	req, err := l.Proto.Get(netconn)
	if err != nil {
		log.Println(cid, "init error", err)
		return
	}

	log.Println(cid, "handle, get wsconn")
	wsconn, err := l.GetWSConn()
	if err != nil {
		log.Println(cid, "cannot connect to ws", err)
		return
	}

	log.Println(cid, "handle, wsconn send 'connect'")
	msg := &common.Message{
		Cmd:     common.CONNECT,
		Cid:     cid,
		Wid:     wsconn.Wid,
		Network: req.Network,
		Address: req.Address,
	}
	err = wsconn.WriteMessage(msg)
	if err != nil {
		log.Println(cid, "handle, wsconn send error", err)
		return
	}

	log.Println(cid, "handle, wait on msg channel")
	conn := &Conn{
		Wid:     wsconn.Wid,
		Cid:     cid,
		MsgChan: make(chan *common.Message, 32),
		Quit:    make(chan interface{}),
		NetConn: netconn,
		WSConn:  wsconn,
	}
	l.Conns.Store(cid, conn)
	select {
	case <-conn.Quit:
		log.Println(cid, "handle, quit before ack")
		return
	case msg = <-conn.MsgChan:
	}

	log.Println(cid, "handle, send ack")
	err = l.Proto.Ack(netconn, msg.Ok, msg.Msg)
	if err != nil {
		log.Println(cid, "handle, ack error", err)
		return
	}

	handleOk = true
	go l.CopyFromWS(conn)
	go l.CopyToWS(conn)
}

func (l *Local) CopyFromWS(conn *Conn) {
	log.Println(conn.Cid, "copy-from-ws, start")
	defer func() {
		log.Println(conn.Cid, "copy-from-ws, close conn")
		conn.NetConn.Close()
		l.Conns.Delete(conn.Cid)
	}()

	for {
		var msg *common.Message
		select {
		case <-conn.Quit:
			log.Println(conn.Cid, "copy-from-ws, 'quit'")
			return
		case msg = <-conn.MsgChan:
		}

		log.Println(conn.Cid, "copy-from-ws, get <=== ws", msg.Cmd, len(msg.Data))
		switch msg.Cmd {
		case common.CLOSE:
			log.Println(conn.Cid, "copy-from-ws, 'close'")
			return
		case common.DATA:
			nw, err := conn.NetConn.Write(msg.Data)
			if err != nil {
				log.Println(conn.Cid, "copy-from-ws, write error", err)
				return
			}
			if nw == 0 {
				log.Println(conn.Cid, "copy-from-ws, close by '0' data")
				return
			}
		}
	}
}

func (l *Local) CopyToWS(conn *Conn) {
	log.Println(conn.Cid, "copy-to-ws, start")
	defer func() {
		log.Println(conn.Cid, "copy-to-ws, close conn")
		conn.NetConn.Close()
		l.Conns.Delete(conn.Cid)
	}()

	buf := make([]byte, BUFSIZE)
	for {
		nr, err := conn.NetConn.Read(buf)
		if err != nil {
			log.Println(conn.Cid, "copy-to-ws, read error", err)
			return
		}

		msg := &common.Message{
			Cmd:     common.DATA,
			Wid:     conn.Wid,
			Cid:     conn.Cid,
			Network: l.Network,
			Address: l.Address,
			Data:    append([]byte{}, buf[:nr]...),
		}
		if nr == 0 {
			msg.Cmd = common.CLOSE
		}

		log.Println(conn.Cid, "copy-to-ws, send ===> ws", msg.Cmd, len(msg.Data))
		err = conn.WSConn.WriteMessage(msg)
		if err != nil {
			log.Println(conn.Cid, "copy-to-ws, write error", err)
			return
		}

		if nr == 0 {
			log.Println(conn.Cid, "copy-to-ws, close by '0' data")
			return
		}
	}
}
