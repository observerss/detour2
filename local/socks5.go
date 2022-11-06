package local

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

const (
	PROTO_SOCKS5   = "socks5"
	SOCKS5_VERSION = 5
	METHOD_NOAUTH  = 0
	SOCKS5_CONNECT = 1
	ADDR_IPV4      = 1
	ADDR_DOMAIN    = 3
	ADDR_IPV6      = 4
)

var (
	NO_ACCEPTABLE_METHODS = []byte{5, 255}
	ACCEPT_METHOD_AUTH    = []byte{5, 0}
	CMD_OK                = []byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}
	CMD_FAILED            = []byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0}
	CMD_NOT_SUPPORTED     = []byte{5, 7, 0, 1, 0, 0, 0, 0, 0, 0}
)

type Socks5Proto struct{}

func (s *Socks5Proto) Get(conn net.Conn) (req *Request, err error) {
	defer func() {
		// if it panics, we don't care
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	buf := make([]byte, BUFFER_SIZE)

	nr, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	if nr < 3 {
		return nil, errors.New("bad request, nr < 3")
	}

	version := buf[0]
	if version != SOCKS5_VERSION {
		conn.Write(NO_ACCEPTABLE_METHODS)
		return nil, fmt.Errorf("bad request, version error %v", version)
	}

	nmethods := buf[1]
	method := -1
	for i := 0; i < int(nmethods); i++ {
		if buf[2+i] == METHOD_NOAUTH {
			method = METHOD_NOAUTH
			break
		}
	}
	if method == -1 {
		conn.Write(NO_ACCEPTABLE_METHODS)
		return nil, errors.New("no acceptable method")
	}
	conn.Write(ACCEPT_METHOD_AUTH)

	// now get the request
	nr, err = conn.Read(buf)
	if err != nil {
		return nil, err
	}
	if nr < 10 {
		return nil, errors.New("bad request, nr < 10")
	}

	if buf[0] != SOCKS5_VERSION {
		conn.Write(CMD_FAILED)
		return nil, errors.New("wrong version")
	}
	if buf[1] != SOCKS5_CONNECT {
		conn.Write(CMD_NOT_SUPPORTED)
		return nil, fmt.Errorf("cmd not supported %v", buf[1])
	}
	var network, address string
	addrType := buf[3]
	switch addrType {
	case ADDR_IPV4:
		network = "tcp"
		address = fmt.Sprintf("%v:%v", net.IP(buf[4:8]), binary.BigEndian.Uint16(buf[8:10]))
	case ADDR_DOMAIN:
		length := buf[4]
		network = "tcp"
		address = fmt.Sprintf("%v:%v", string(buf[5:5+length]), binary.BigEndian.Uint16(buf[5+length:7+length]))
	case ADDR_IPV6:
		network = "tcp6"
		address = fmt.Sprintf("%v:%v", net.IP(buf[4:20]), binary.BigEndian.Uint16(buf[20:22]))
	default:
		return nil, errors.New("unknown addr type")
	}

	return &Request{Network: network, Address: address}, nil
}

func (s *Socks5Proto) Ack(conn net.Conn, ok bool, msg string) error {
	if ok {
		_, err := conn.Write(CMD_OK)
		return err
	} else {
		_, err := conn.Write(CMD_FAILED)
		return err
	}
}
