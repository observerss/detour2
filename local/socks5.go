package local

import (
	"log"
	"net"
	"strconv"
)

type Socks5Protocol struct {
	Conn net.Conn
}

func (s *Socks5Protocol) Init() *Remote {
	buf := make([]byte, DOWNSTREAM_BUFSIZE)
	_, err := s.Conn.Read(buf)

	if err != nil {
		log.Println("init read error:", err)
		return nil
	}

	if buf[0] != 5 {
		log.Println("only socks5 supported, but got:", buf[0])
		s.Conn.Write([]byte{5, 255})
		return nil
	}

	if buf[1] == 0 {
		log.Println("0 methods are not supported")
		s.Conn.Write([]byte{5, 255})
		return nil
	}

	methods := buf[2 : 2+buf[1]]
	noauth := false
	for method := range methods {
		if method == 0 {
			noauth = true
			break
		}
	}
	if !noauth {
		log.Println("only noauth is supported")
		s.Conn.Write([]byte{5, 255})
		return nil
	}

	// no auth
	s.Conn.Write([]byte{5, 0})

	_, err = s.Conn.Read(buf)
	if err != nil {
		log.Println("request read error:", err)
		return nil
	}

	cmd := buf[1]
	if cmd != 1 {
		log.Println("only cmd=connect is supproted")
		return nil
	}

	var network, address string
	addrtype := buf[3]
	switch addrtype {
	case 1:
		network = "tcp"
		address = net.IP(buf[4:8]).String() + ":" + strconv.Itoa(256*int(buf[8])+int(buf[9]))
	case 3:
		network = "tcp"
		length := buf[4]
		address = string(buf[5:5+length]) + ":" + strconv.Itoa(256*int(buf[5+length])+int(buf[6+length]))
	case 4:
		network = "tcp6"
		address = net.IP(buf[4:20]).String() + ":" + strconv.Itoa(256*int(buf[20])+int(buf[21]))
	}
	return &Remote{Network: network, Address: address}
}

func (s *Socks5Protocol) Bind(err error) error {
	if err != nil {
		s.Conn.Write([]byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0})
		return err
	}
	_, err = s.Conn.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
	if err != nil {
		return err
	}
	return nil
}
