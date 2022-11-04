package server

import (
	"log"
	"net"
)

type Server struct {
	Network string
	Address string
}

func NewServer(network string, address string) *Server {
	server := &Server{
		Network: network,
		Address: address,
	}
	return server
}

func (s *Server) Run() {
	listen, err := net.Listen(s.Network, s.Address)
	if err != nil {
		log.Fatal(err)
	}

	for {
		conn, err := listen.Accept()
		if err != nil {
			log.Println("server, accept", err)
			continue
		}

		HandleConn(conn)
	}
}

func HandleConn(conn net.Conn) {

}
