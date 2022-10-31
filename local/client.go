package local

import (
	"detour/relay"
	"errors"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var CLIENTID, _ = uuid.NewUUID()
var TIME_TO_LIVE = time.Second * 56
var lock = sync.Mutex{}

type Client struct {
	Password      string
	RemoteUrls    []string
	RemoteConns   []*RemoteConn
	CanConnect    map[string]bool
	ConnIdRemotes map[uuid.UUID]int
	currIdx       int
}

type RemoteConn struct {
	Url       string
	Connected bool
	Conn      *websocket.Conn
	CreatedAt time.Time
	Switching bool
	Password  string
	Channels  map[uuid.UUID]*RemoteConnChannels
	WriteChan chan *WriteData
}

type WriteData struct {
	ConnId uuid.UUID
	Data   *relay.RelayData
}

type RemoteConnChannels struct {
	ReadChan chan *relay.RelayData
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
			Url:       url,
			Connected: false,
			Switching: false,
		})
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
	if !remoteConn.Connected {
		err = c.ConnectRemote(cp.ConnId, remoteConn)
		if err != nil {
			return nil, err
		}
	}

	_, ok := remoteConn.Channels[cp.ConnId]
	if !ok {
		remoteConn.Channels[cp.ConnId] = &RemoteConnChannels{ReadChan: make(chan *relay.RelayData)}
	}

	// switch connection if TIME_TO_LIVE has passed
	if time.Since(remoteConn.CreatedAt) > TIME_TO_LIVE && !remoteConn.Switching {
		remoteConn.Switching = true
		go c.SwitchRemote(remoteConn)
	}

	return remoteConn, nil
}

func (c *Client) FindIdx(connid uuid.UUID) (int, error) {
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
	return idx, nil
}

func (c *Client) ConnectRemote(connid uuid.UUID, remoteConn *RemoteConn) error {
	dailer := websocket.Dialer{}
	dailer.HandshakeTimeout = time.Second * 3

	conn, _, err := dailer.Dial(remoteConn.Url, nil)
	if err != nil {
		c.CanConnect[remoteConn.Url] = false
		go c.RetryUrl(remoteConn.Url)
		return errors.New(err.Error() + ", url=" + remoteConn.Url)
	}

	c.CanConnect[remoteConn.Url] = true
	remoteConn.Conn = conn
	remoteConn.Connected = true
	remoteConn.Password = c.Password
	remoteConn.CreatedAt = time.Now()
	remoteConn.Channels = make(map[uuid.UUID]*RemoteConnChannels)
	remoteConn.WriteChan = make(chan *WriteData)

	go c.RemoteReader(remoteConn)
	go c.RemoteWriter(remoteConn)

	return nil
}

func (c *Client) SwitchRemote(remoteConn *RemoteConn) {
	dailer := websocket.Dialer{}
	dailer.HandshakeTimeout = time.Second * 3

	conn, _, err := dailer.Dial(remoteConn.Url, nil)
	if err != nil {
		log.Println("GetConn error: switch connect failed", err, remoteConn.Url)
		remoteConn.Connected = false
		remoteConn.Conn.Close()
		c.CanConnect[remoteConn.Url] = false
		return
	}

	c.CanConnect[remoteConn.Url] = true
	remoteConn.Conn = conn
	remoteConn.Connected = true
	remoteConn.Switching = false
	remoteConn.Password = c.Password
	remoteConn.CreatedAt = time.Now()
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
	for {
		// log.Println("loop reader start", conn.Url)

		if !conn.Connected {
			log.Println("client reader wait for connect...")
			time.Sleep(time.Second)
			continue
		}

		mt, data, err := conn.Conn.ReadMessage()

		if err != nil {
			log.Println("client read error:", conn.Url, err)
			return
		}

		if mt != websocket.BinaryMessage {
			log.Println("client message type error:", conn.Url, mt)
			return
		}

		msg, err := relay.Unpack(data, c.Password)
		if err != nil {
			log.Println("client message unpack error", conn.Url, err)
			return
		}

		idx, err := c.FindIdx(msg.Pair.ConnId)
		if err != nil {
			log.Println("client findidx error:", err)
			return
		}

		remoteConn := c.RemoteConns[idx]
		chs, ok := remoteConn.Channels[msg.Pair.ConnId]
		if !ok {
			log.Println("client read can't find channel", msg.Pair.ConnId)
			return
		}

		// log.Println("reader got data:", msg.Data)
		chs.ReadChan <- msg.Data
	}
}

func (c *Client) RemoteWriter(conn *RemoteConn) {
	for {
		// log.Println("loop writer start", conn.Url, conn.WriteChan)

		if !conn.Connected {
			log.Println("client writer wait for connect...", conn.Url)
			time.Sleep(time.Second)
			continue
		}

		wd := <-conn.WriteChan

		if wd == nil {
			log.Println("client writer quit", conn.Url)
			break
		}

		// log.Println("writer got data:", wd.Data)

		msg := &relay.RelayMessage{Pair: &relay.ConnPair{ClientId: CLIENTID, ConnId: wd.ConnId}, Data: wd.Data}

		err := conn.Conn.WriteMessage(websocket.BinaryMessage, relay.Pack(msg, c.Password))
		if err != nil {
			log.Println("client writer error:", conn.Url, err)
			break
		}
	}
}

// Connect makes a CONNECT call to server
func (c *Client) Connect(cp *relay.ConnPair, remote *Remote) (*ConnByPair, error) {
	conn, err := c.GetConn(cp)
	if err != nil {
		return nil, err
	}

	// wait for coroutines to set up
	time.Sleep(time.Millisecond * 10)

	connByPair := &ConnByPair{Pair: cp, Conn: conn}

	connByPair.WriteConnect(remote.Network, remote.Address)

	data := <-conn.Channels[cp.ConnId].ReadChan

	if !data.OK {
		return nil, errors.New("remote error: " + data.MSG)
	}

	return &ConnByPair{Pair: cp, Conn: conn}, nil
}

func (c *ConnByPair) Read(buf []byte) (int, error) {
	ch, ok := c.Conn.Channels[c.Pair.ConnId]
	if !ok {
		return 0, errors.New("reader already closed")
	}

	data := <-ch.ReadChan

	switch data.CMD {
	case relay.RECONNECT:
		// remote connection has lost, should reconnect
		// currently we just report an error
		return 0, errors.New("remote connection lost, reconnect first")
	case relay.SWITCH:
		// switch to new connection, which should be updated in RemoteConn already, do nothing
		log.Println("switch(noop)")
		return c.Read(buf)
	case relay.DATA:
		copied := copy(buf, data.Data)
		// log.Println("copied", copied)
		if copied < len(data.Data) {
			log.Println("read buffer overflow, this should not happen, incress receive buffer size!")
		}
		return copied, nil
	default:
		return 0, errors.New("unexpected cmd: " + strconv.Itoa(int(data.CMD)))
	}
}

func (c *ConnByPair) WriteConnect(network string, address string) {
	c.Conn.WriteChan <- &WriteData{ConnId: c.Pair.ConnId, Data: &relay.RelayData{CMD: relay.CONNECT, Network: network, Address: address}}
}

func (c *ConnByPair) Write(data []byte) (int, error) {
	c.Conn.WriteChan <- &WriteData{ConnId: c.Pair.ConnId, Data: &relay.RelayData{CMD: relay.DATA, Data: data}}
	return len(data), nil
}

func (c *ConnByPair) Close() error {
	delete(c.Conn.Channels, c.Pair.ConnId)
	return nil
}

func (r *RemoteConn) writeMessage(c *websocket.Conn, msg *relay.RelayMessage) error {
	// if msg.Data.CMD == relay.DATA {
	// 	log.Println("write:", len(msg.Data.Data))
	// }

	lock.Lock()
	defer lock.Unlock()
	return c.WriteMessage(websocket.BinaryMessage, relay.Pack(msg, r.Password))
}

func newConnectMessage(cp *relay.ConnPair, network string, address string) *relay.RelayMessage {
	return &relay.RelayMessage{
		Pair: cp,
		Data: &relay.RelayData{CMD: relay.CONNECT, Network: network, Address: address},
	}
}
