package local

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/observerss/detour2/common"
	"github.com/observerss/detour2/logger"
)

const (
	BUFFER_SIZE  = 16 * 1024
	READ_TIMEOUT = 60 // sec
)

type Local struct {
	Network  string
	Address  string
	Packer   *common.Packer
	Proto    Proto
	WSConns  map[string]*WSConn // url => WSConn
	Conns    sync.Map           // Cid => Conn
	Listener net.Listener
}
type Conn struct {
	Cid     string
	Wid     string
	Quit    chan interface{}
	Network string
	Address string
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
		WSConns: make(map[string]*WSConn),
	}
	switch lconf.Proto {
	case PROTO_SOCKS5:
		local.Proto = &Socks5Proto{}
	case PROTO_HTTP:
		local.Proto = &HTTPProto{}
	default:
		logger.Error.Fatalln("proto", lconf.Proto, "not supported")
	}
	for _, url := range urls {
		wid, _ := common.GenerateRandomStringURLSafe(3)
		local.WSConns[url] = NewWSConn(url, wid, local)
	}
	return local
}

func (l *Local) RunLocal() error {
	defer func() {
		logger.Info.Println("Local server stopped.")
	}()

	listen, err := net.Listen(l.Network, l.Address)
	l.Listener = listen
	if err != nil {
		return err
	}

	logger.Info.Println("Listening on " + l.Network + "://" + l.Address)

	// background wsconn puller & keeper
	for _, wsconn := range l.WSConns {
		go wsconn.WebsocketPuller()
	}

	for {
		conn, err := listen.Accept()
		if err != nil {
			logger.Error.Println("run, accept error", err)
			break
		}

		go l.HandleConn(conn)
	}

	return nil
}

func (l *Local) StopLocal() {
	defer func() {
		if r := recover(); r != nil {
			logger.Error.Println(r)
		}
	}()
	if l.Listener != nil {
		l.Listener.Close()
	}
	for _, wsconn := range l.WSConns {
		c := wsconn.WSConn
		if c != nil {
			c.Close()
		}
	}
	l.Conns.Range(func(key, value any) bool {
		c := value.(*Conn).NetConn
		if c != nil {
			c.Close()
		}
		return true
	})
}

func (l *Local) HandleConn(netconn net.Conn) {
	cid, _ := common.GenerateRandomStringURLSafe(6)
	handleOk := false
	defer func() {
		if !handleOk {
			logger.Error.Println(cid, "handle, close conn")
			netconn.Close()
			l.Conns.Delete(cid)
		}
	}()

	logger.Debug.Println(cid, "handle, init")
	req, err := l.Proto.Get(netconn)
	if err != nil {
		logger.Debug.Println(cid, "init error", err)
		return
	}
	logger.Info.Println(cid, "handle, get", req.Address)

	logger.Debug.Println(cid, "handle, get wsconn")
	wsconn, err := l.GetWSConn()
	if err != nil {
		logger.Debug.Println(cid, "cannot connect to ws", err)
		return
	}

	logger.Debug.Println(cid, "handle, wsconn send 'connect'")
	msg := &common.Message{
		Cmd:     common.CONNECT,
		Cid:     cid,
		Wid:     wsconn.Wid,
		Network: req.Network,
		Address: req.Address,
	}
	err = wsconn.WriteMessage(msg)
	if err != nil {
		logger.Debug.Println(cid, "handle, wsconn send error", err)
		return
	}

	logger.Debug.Println(cid, "handle, wait on msg channel")
	conn := &Conn{
		Wid:     wsconn.Wid,
		Cid:     cid,
		MsgChan: make(chan *common.Message, 32),
		Quit:    make(chan interface{}),
		Network: msg.Network,
		Address: msg.Address,
		NetConn: netconn,
		WSConn:  wsconn,
	}
	l.Conns.Store(cid, conn)
	select {
	case <-conn.Quit:
		logger.Debug.Println(cid, "handle, quit before ack")
		return
	case msg = <-conn.MsgChan:
	}

	logger.Debug.Println(cid, "handle, send ack", msg.Ok, msg.Network, msg.Address)
	err = l.Proto.Ack(netconn, msg.Ok, msg.Msg, req)
	if err != nil {
		logger.Debug.Println(cid, "handle, ack error", err)
		return
	}
	if !msg.Ok {
		logger.Debug.Println(cid, "open connection failed", msg.Msg)
		return
	}

	// flush the request data
	if req.More {
		buf := make([]byte, BUFFER_SIZE)
		for {
			nr, err := req.Reader.Read(buf)
			if err != nil {
				logger.Debug.Println(cid, "handle, send more error", err)
				return
			}

			msg := &common.Message{
				Cmd:     common.DATA,
				Wid:     conn.Wid,
				Cid:     conn.Cid,
				Network: conn.Network,
				Address: conn.Address,
				Data:    append([]byte{}, buf[:nr]...),
			}

			logger.Debug.Println(conn.Cid, "copy-to-ws, read <=== local", msg.Cmd, len(msg.Data))
			err = conn.WSConn.WriteMessage(msg)
			if err != nil {
				logger.Debug.Println(conn.Cid, "copy-to-ws, write error", err)
				return
			}

			logger.Debug.Println(conn.Cid, "copy-to-ws, sent ===> ws", nr)

			if nr < BUFFER_SIZE {
				break
			}
		}
	}

	handleOk = true
	logger.Debug.Println(conn.Cid, "handle, ok")

	go l.CopyFromWS(conn)
	go l.CopyToWS(conn)
}

func (l *Local) CopyFromWS(conn *Conn) {
	defer func() {
		logger.Info.Println(conn.Cid, "copy-from-ws, close conn")
		conn.NetConn.Close()
		l.Conns.Delete(conn.Cid)
	}()

	logger.Debug.Println(conn.Cid, "copy-from-ws, start")

	for {
		var msg *common.Message
		select {
		case <-conn.Quit:
			logger.Debug.Println(conn.Cid, "copy-from-ws, 'quit'")
			return
		case msg = <-conn.MsgChan:
		}

		logger.Debug.Println(conn.Cid, "copy-from-ws, get <=== queue", msg.Cmd, len(msg.Data))
		switch msg.Cmd {
		case common.CLOSE:
			logger.Debug.Println(conn.Cid, "copy-from-ws, 'close'")
			return
		case common.DATA:
			nw, err := conn.NetConn.Write(msg.Data)
			if err != nil {
				logger.Debug.Println(conn.Cid, "copy-from-ws, write error", err)
				return
			}
			if nw == 0 {
				logger.Debug.Println(conn.Cid, "copy-from-ws, close by '0' data")
				return
			}
			logger.Debug.Println(conn.Cid, "copy-from-ws, written ===> local", nw)
		}
	}
}

func (l *Local) CopyToWS(conn *Conn) {
	defer func() {
		logger.Info.Println(conn.Cid, "copy-to-ws, close conn")
		conn.NetConn.Close()
		l.Conns.Delete(conn.Cid)
		if !IsClosed(conn.Quit) {
			close(conn.Quit)
		}
	}()

	logger.Debug.Println(conn.Cid, "copy-to-ws, start")

	buf := make([]byte, BUFFER_SIZE)
	for {
		conn.NetConn.SetReadDeadline(time.Now().Add(time.Second * READ_TIMEOUT))
		nr, err := conn.NetConn.Read(buf)
		if err != nil {
			logger.Debug.Println(conn.Cid, "copy-to-ws, read error", err)
			return
		}

		msg := &common.Message{
			Cmd:     common.DATA,
			Wid:     conn.Wid,
			Cid:     conn.Cid,
			Network: conn.Network,
			Address: conn.Address,
			Data:    append([]byte{}, buf[:nr]...),
		}
		if nr == 0 {
			msg.Cmd = common.CLOSE
		}

		logger.Debug.Println(conn.Cid, "copy-to-ws, read <=== local", msg.Cmd, len(msg.Data))
		err = conn.WSConn.WriteMessage(msg)
		if err != nil {
			logger.Debug.Println(conn.Cid, "copy-to-ws, write error", err)
			return
		}

		logger.Debug.Println(conn.Cid, "copy-to-ws, sent ===> ws", nr)

		if nr == 0 {
			logger.Debug.Println(conn.Cid, "copy-to-ws, close by '0' data")
			return
		}
	}
}
