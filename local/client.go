package local

import (
	"detour/relay"
	"errors"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var CLIENTID, _ = uuid.NewUUID()
var TIME_TO_LIVE = time.Second * 56
var password = "yb160101"

type Client struct {
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
}

func NewClient(remoteUrls []string) *Client {
	if len(remoteUrls) == 0 {
		log.Fatal("remote urls should not be empty!")
		os.Exit(1)
	}
	client := &Client{currIdx: 0, RemoteUrls: remoteUrls}
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
func (c *Client) GetConn(cp *relay.ConnPair) (*websocket.Conn, error) {
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
		err = c.ConnectRemote(remoteConn)
		if err != nil {
			return nil, err
		}
	}

	// switch connection if TIME_TO_LIVE has passed
	if time.Since(remoteConn.CreatedAt) > TIME_TO_LIVE && !remoteConn.Switching {
		remoteConn.Switching = true
		go c.SwitchRemote(remoteConn)
	}

	return remoteConn.Conn, nil
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

func (c *Client) ConnectRemote(remoteConn *RemoteConn) error {
	dailer := websocket.Dialer{}
	dailer.HandshakeTimeout = time.Second * 3

	conn, _, err := dailer.Dial(remoteConn.Url, nil)
	if err != nil {
		log.Println("ConnectRemote error: connect failed", err, remoteConn.Url)
		c.CanConnect[remoteConn.Url] = false
		go c.RetryUrl(remoteConn.Url)
		return err
	}

	c.CanConnect[remoteConn.Url] = true
	remoteConn.Conn = conn
	remoteConn.Connected = true
	remoteConn.CreatedAt = time.Now()

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
	remoteConn.Switching = false
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
		log.Println("retry failed:", url)

		timeout = timeout * 2
		if timeout > maxTimeout {
			timeout = maxTimeout
		}
	}
}

// Connect makes a CONNECT call to server
func (c *Client) Connect(cp *relay.ConnPair, remote *Remote) error {
	conn, err := c.GetConn(cp)
	if err != nil {
		return err
	}

	err = writeMessage(conn, newConnectMessage(cp, remote.Network, remote.Address))
	if err != nil {
		return err
	}

	msg, err := readMessage(conn)
	if err != nil {
		return err
	}

	if !msg.Data.OK {
		return errors.New("remote error: " + msg.Data.MSG)
	}

	return nil
}

func (c *Client) GetReader(cp *relay.ConnPair) io.Reader {
	// TODO: implement this
	return nil
}

func (c *Client) GetWriter(cp *relay.ConnPair) io.Writer {
	// TODO: implement this
	return nil
}

func (c *Client) Close(cp *relay.ConnPair) error {
	// TODO: implement this
	return nil
}

func writeMessage(c *websocket.Conn, msg *relay.RelayMessage) error {
	return c.WriteMessage(websocket.BinaryMessage, relay.Pack(msg, password))
}

func readMessage(c *websocket.Conn) (*relay.RelayMessage, error) {
	mt, data, err := c.ReadMessage()
	if err != nil {
		return nil, err
	}
	switch mt {
	case websocket.BinaryMessage:
		msg, err := relay.Unpack(data, password)
		return msg, err
	default:
		return nil, errors.New("unexpected message type: " + strconv.Itoa(mt))
	}
}

// func newErrorMessage(msg *relay.RelayMessage, err error) *relay.RelayMessage {
// 	return &relay.RelayMessage{
// 		Pair: msg.Pair,
// 		Data: &relay.RelayData{CMD: relay.CONNECT, OK: false, MSG: err.Error()},
// 	}
// }

// func newOKMessage(msg *relay.RelayMessage) *relay.RelayMessage {
// 	return &relay.RelayMessage{
// 		Pair: msg.Pair,
// 		Data: &relay.RelayData{CMD: relay.CONNECT, OK: true},
// 	}
// }

func newConnectMessage(cp *relay.ConnPair, network string, address string) *relay.RelayMessage {
	return &relay.RelayMessage{
		Pair: cp,
		Data: &relay.RelayData{CMD: relay.CONNECT, Network: network, Address: address},
	}
}

// func newDataMessage(msg *relay.RelayMessage, data []byte) *relay.RelayMessage {
// 	return &relay.RelayMessage{
// 		Pair: msg.Pair,
// 		Data: &relay.RelayData{CMD: relay.DATA, Data: data},
// 	}
// }
