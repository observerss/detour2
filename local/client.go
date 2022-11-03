package local

import (
	"detour/relay"
	"errors"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var CLIENTID, _ = uuid.NewUUID()
var TIME_TO_LIVE = time.Second * 56

type Client struct {
	Password      string
	RemoteUrls    []string
	RemoteConns   []*RemoteConn
	CanConnect    map[string]bool
	ConnIdRemotes map[uuid.UUID]int
	currIdx       int
	MapLock       sync.RWMutex
	WSLock        sync.RWMutex
}

type RemoteConn struct {
	Client       *Client
	Url          string
	Connected    bool
	Running      bool
	Conn         *websocket.Conn
	CreatedAt    time.Time
	Switching    bool
	Password     string
	ReadChannels map[uuid.UUID]chan *relay.RelayData
	WriteChan    chan *WriteData
}

type WriteData struct {
	ConnId uuid.UUID
	Data   *relay.RelayData
}

type ConnByPair struct {
	Pair *relay.ConnPair
	Conn *RemoteConn
}

func NewClient(remoteUrls []string, password string) *Client {
	if len(remoteUrls) == 0 {
		log.Fatal("remote urls should not be empty!")
		os.Exit(1)
	}

	client := &Client{
		currIdx: 0, RemoteUrls: remoteUrls, Password: password,
		CanConnect: make(map[string]bool), ConnIdRemotes: make(map[uuid.UUID]int),
	}
	for _, url := range remoteUrls {
		client.RemoteConns = append(client.RemoteConns, &RemoteConn{
			Client:    client,
			Url:       url,
			Connected: false,
			Switching: false,
		})

		// uncomment this to connect on startup
		// client.ConnectRemote(client.RemoteConns[0])

		client.CanConnect[url] = true
	}
	return client
}

// GetConn lazily get connection to a server
// a connection may only persist within around 50-60 seconds
// after that, we deliberately disconnect and make a new connection
func (c *Client) GetConn(cp *relay.ConnPair) (*RemoteConn, error) {

	if cp.ClientId != CLIENTID {
		log.Println("GetConn error: ClientId does not match", CLIENTID, cp.ClientId)
		return nil, errors.New("ClientId does not match")
	}
	idx, err := c.FindIdx(cp.ConnId)
	if err != nil {
		log.Println("GetConn error:", err)
	}

	remoteConn := c.RemoteConns[idx]

	// do connect if not connected
	if !remoteConn.Running {
		err := c.ConnectRemote(remoteConn)
		if err != nil {
			return nil, err
		}
		remoteConn.Running = true
	}

	c.MapLock.Lock()
	_, ok := remoteConn.ReadChannels[cp.ConnId]
	if !ok {
		remoteConn.ReadChannels[cp.ConnId] = make(chan *relay.RelayData, 1)
	}
	c.MapLock.Unlock()

	// switch connection if TIME_TO_LIVE has passed
	if time.Since(remoteConn.CreatedAt) > TIME_TO_LIVE {
		remoteConn.Switching = true
		err := c.SwitchRemote(remoteConn)
		if err != nil {
			return nil, err
		}
		log.Println("switched to new connection")
	}

	return remoteConn, nil
}

func (c *Client) FindIdx(connid uuid.UUID) (int, error) {
	c.MapLock.Lock()
	idx, ok := c.ConnIdRemotes[connid]
	if !ok {
		startIdx := c.currIdx
		for {
			c.ConnIdRemotes[connid] = c.currIdx
			idx = c.currIdx
			if c.CanConnect[c.RemoteUrls[idx]] {
				break
			}

			c.currIdx += 1
			if c.currIdx >= len(c.RemoteConns) {
				c.currIdx = 0
			}

			if c.currIdx == startIdx {
				return 0, errors.New("no remote url usable")
			}
		}
	}
	c.MapLock.Unlock()
	return idx, nil
}

func (c *Client) ConnectRemote(remoteConn *RemoteConn) error {
	dailer := websocket.Dialer{}
	dailer.HandshakeTimeout = time.Second * 3

	conn, _, err := dailer.Dial(remoteConn.Url, nil)

	c.WSLock.Lock()
	defer c.WSLock.Unlock()

	if err != nil {
		c.CanConnect[remoteConn.Url] = false
		remoteConn.Connected = false
		go c.RetryUrl(remoteConn.Url)
		return errors.New(err.Error() + ", url=" + remoteConn.Url)
	}

	c.CanConnect[remoteConn.Url] = true
	remoteConn.Conn = conn
	remoteConn.Connected = true
	remoteConn.Password = c.Password
	remoteConn.CreatedAt = time.Now()

	remoteConn.ReadChannels = make(map[uuid.UUID]chan *relay.RelayData)
	remoteConn.WriteChan = make(chan *WriteData)
	go c.RemoteReader(remoteConn)
	go c.RemoteWriter(remoteConn)

	return nil
}

func (c *Client) SwitchRemote(remoteConn *RemoteConn) error {
	dailer := websocket.Dialer{}
	dailer.HandshakeTimeout = time.Second * 3

	conn, _, err := dailer.Dial(remoteConn.Url, nil)
	c.WSLock.Lock()
	defer c.WSLock.Unlock()

	if err != nil {
		log.Println("GetConn error: switch connect failed", err, remoteConn.Url)
		remoteConn.Connected = false
		remoteConn.Conn.Close()
		c.CanConnect[remoteConn.Url] = false
		return err

	}

	log.Println("switch close old conn")
	remoteConn.Conn.Close()

	c.CanConnect[remoteConn.Url] = true
	remoteConn.Conn = conn
	remoteConn.Connected = true
	remoteConn.Switching = false
	remoteConn.Password = c.Password
	remoteConn.CreatedAt = time.Now()

	return nil
}

func (c *Client) RetryUrl(url string) {
	timeout := 10 * time.Second
	maxTimeout := 60 * time.Second

	for {
		time.Sleep(timeout)

		dailer := websocket.Dialer{}
		dailer.HandshakeTimeout = time.Second * 3

		conn, _, err := dailer.Dial(url, nil)
		if err == nil {
			conn.Close()
			c.CanConnect[url] = true
			log.Println("retry ok:", url)
			break
		}

		c.CanConnect[url] = false
		log.Println("retry failed, url=" + url)

		timeout = timeout * 2
		if timeout > maxTimeout {
			timeout = maxTimeout
		}
	}
}

func (c *Client) RemoteReader(conn *RemoteConn) {
	defer func() {
		log.Println("reader closed", conn.Url)
		conn.Connected = false
	}()

	closeCount := 0

	for {
		log.Println("reader waiting", &conn.Conn)

		if !conn.Connected {
			time.Sleep(time.Second)
			log.Println("reader wait for connect...")
			err := c.SwitchRemote(conn)
			if err != nil {
				continue
			}
			c.CanConnect[conn.Url] = true
			continue
		}

		mt, data, err := func() (int, []byte, error) {
			defer func() {
				if r := recover(); r != nil {
					log.Println("reader read message recovered from", r)
				}
			}()
			mt := 0
			var data []byte
			var err error
			mt, data, err = conn.Conn.ReadMessage()
			return mt, data, err
		}()

		if err != nil {
			closeCount += 1
			if closeCount > 3 {
				c.MapLock.Lock()
				conn.Connected = false
				c.MapLock.Unlock()
			}
			if !strings.Contains(err.Error(), "1006") {
				log.Println("reader error:", conn.Url, err)
			}
			continue
		}

		closeCount = 0

		if mt != websocket.BinaryMessage {
			log.Println("reader message type error:", conn.Url, mt)
			continue
		}

		msg, err := relay.Unpack(data, c.Password)
		if err != nil {
			log.Println("reader message unpack error", conn.Url, err)
			continue
		}

		// log.Println("reader got", msg.Pair.ConnId.String()[:8], msg.Data.CMD, len(msg.Data.Data))

		idx, err := c.FindIdx(msg.Pair.ConnId)
		if err != nil {
			log.Println("reader findidx error:", err)
			continue
		}

		remoteConn := c.RemoteConns[idx]
		c.MapLock.Lock()
		chr, ok := remoteConn.ReadChannels[msg.Pair.ConnId]
		c.MapLock.Unlock()
		if !ok {
			// channel has been closed, data dropped
			// log.Println("reader can't find channel", msg.Pair.ConnId)
			continue
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					// send on closed channel, don't panic
					// the msg can go nowhere, just drop it
					log.Println("recover from panic", r)
				}
			}()
			chr <- msg.Data
		}()

	}
}

func (c *Client) RemoteWriter(conn *RemoteConn) {
	defer func() {
		log.Println("writer closed", conn.Url)
		conn.Connected = false
	}()

	for {
		log.Println("writer waiting", &conn.Conn)

		if !conn.Connected {
			time.Sleep(time.Second)
			log.Println("writer wait for connect...")
			err := c.SwitchRemote(conn)
			if err != nil {
				continue
			}
			c.CanConnect[conn.Url] = true
			continue
		}

		c.MapLock.RLock()
		connected := conn.Connected
		c.MapLock.RUnlock()
		if !connected {
			log.Println("writer wait for connect...", conn.Url)
			time.Sleep(time.Second)
			continue
		}

		wd := <-conn.WriteChan

		// log.Println("writer got", wd.ConnId.String()[:8], wd.Data.CMD, len(wd.Data.Data))

		msg := &relay.RelayMessage{Pair: &relay.ConnPair{ClientId: CLIENTID, ConnId: wd.ConnId}, Data: wd.Data}

		c.WSLock.Lock()
		err := conn.Conn.WriteMessage(websocket.BinaryMessage, relay.Pack(msg, c.Password))
		c.WSLock.Unlock()

		if err != nil {
			log.Println("writer error:", conn.Url, err)
			conn.Connected = false
			continue
		}
	}
}

// Connect makes a CONNECT call to server
func (c *Client) Connect(cp *relay.ConnPair, remote *Remote) (*ConnByPair, error) {
	log.Println("connect getconn")
	conn, err := c.GetConn(cp)
	log.Println("connect getconn ok")
	if err != nil {
		return nil, err
	}

	// wait for coroutines to set up
	time.Sleep(time.Millisecond * 10)

	connByPair := &ConnByPair{Pair: cp, Conn: conn}

	connByPair.WriteConnect(remote.Network, remote.Address)

	c.MapLock.Lock()
	chr := conn.ReadChannels[cp.ConnId]
	c.MapLock.Unlock()

	log.Println("connect wait remote response", cp.ConnId.String()[:8], chr)
	data := <-chr
	log.Println("connect wait remote response ok", cp.ConnId.String()[:8])

	if !data.OK {
		return nil, errors.New("connect remote error: " + data.MSG + " " + cp.ConnId.String()[:8])
	}

	return connByPair, nil
}

func (c *ConnByPair) Read(buf []byte) (int, error) {
	c.Conn.Client.MapLock.Lock()
	chr, ok := c.Conn.ReadChannels[c.Pair.ConnId]
	c.Conn.Client.MapLock.Unlock()
	if !ok {
		return 0, errors.New("reader already closed" + c.Pair.ConnId.String()[:8])
	}

	log.Println("block on read", c.Pair.ConnId.String()[:8])
	data, more := <-chr

	if data == nil || !more {
		log.Println("reader manualy closed", c.Pair.ConnId.String()[:8])
		return 0, io.EOF
	}

	switch data.CMD {
	case relay.RECONNECT:
		// we must delete chr, otherwise it will block the Close method
		c.Conn.Client.MapLock.Lock()
		delete(c.Conn.ReadChannels, c.Pair.ConnId)
		c.Conn.Client.MapLock.Unlock()

		return 0, errors.New("remote connection lost, reconnect first")
	case relay.SWITCH:
		// switch to new connection, which should be updated in RemoteConn already, do nothing
		log.Println("switch(noop)")
		return c.Read(buf)

	case relay.DATA:
		copied := copy(buf, data.Data)
		// log.Println("copied", copied)
		if copied < len(data.Data) {
			log.Println("read buffer overflow, this should not happen, incress receive buffer size! " + c.Pair.ConnId.String()[:8])
		}
		if copied == 0 {
			return 0, io.EOF
		}
		return copied, nil
	default:
		return 0, errors.New("unexpected cmd: " + strconv.Itoa(int(data.CMD)) + c.Pair.ConnId.String()[:8])
	}
}

func (c *ConnByPair) WriteConnect(network string, address string) {
	log.Println("write to", c.Pair.ConnId.String()[:8], "connect", network, address)
	c.Conn.WriteChan <- &WriteData{ConnId: c.Pair.ConnId, Data: &relay.RelayData{CMD: relay.CONNECT, Network: network, Address: address}}
}

func (c *ConnByPair) Write(data []byte) (int, error) {
	log.Println("write to", c.Pair.ConnId.String()[:8], len(data))
	c.Conn.WriteChan <- &WriteData{ConnId: c.Pair.ConnId, Data: &relay.RelayData{CMD: relay.DATA, Data: append([]byte{}, data...)}}
	return len(data), nil
}

func (c *ConnByPair) Close() error {
	defer func() {
		r := recover()
		if r != nil {
			log.Println("recovered from", r)
		}
	}()
	if c != nil && c.Conn != nil && c.Conn.ReadChannels != nil && c.Pair != nil {
		c.Conn.Client.MapLock.RLock()
		chr, ok := c.Conn.ReadChannels[c.Pair.ConnId]
		c.Conn.Client.MapLock.RUnlock()
		if ok {
			log.Println("send nil to", c.Pair.ConnId.String()[:8])
			close(chr)
			log.Println("nil sent", c.Pair.ConnId.String()[:8])
		}
		c.Conn.Client.MapLock.Lock()
		delete(c.Conn.ReadChannels, c.Pair.ConnId)
		c.Conn.Client.MapLock.Unlock()
	}
	if c != nil && c.Conn != nil && c.Conn.Client != nil && c.Conn.Client.ConnIdRemotes != nil && c.Pair != nil {
		c.Conn.Client.MapLock.Lock()
		delete(c.Conn.Client.ConnIdRemotes, c.Pair.ConnId)
		c.Conn.Client.MapLock.Unlock()
	}
	return nil
}
