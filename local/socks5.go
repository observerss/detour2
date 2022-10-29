package local

import (
	"log"
	"net"
	"os"
	"sync"
	"time"
)

type Server struct {
	listener net.Listener
	quit     chan interface{}
	wg       sync.WaitGroup
}

func NewServer(network string, address string) *Server {

	listener, err := net.Listen(network, address)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	server := &Server{
		quit:     make(chan interface{}),
		listener: listener,
	}

	server.wg.Add(1)

	// go server.Serve()
	return server
}

func (s *Server) Serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				log.Println("accept error", err)
			}
		} else {
			s.wg.Add(1)
			go func() {
				go handleIncomingRequest(conn)
				s.wg.Done()
			}()
		}
	}
}

func handleIncomingRequest(conn net.Conn) {
	// store incoming data
	buffer := make([]byte, 1024)
	_, err := conn.Read(buffer)
	if err != nil {
		log.Fatal(err)
	}
	// respond
	time := time.Now().Format("Monday, 02-Jan-06 15:04:05 MST")
	conn.Write([]byte("Hi back!\n"))
	conn.Write([]byte(time))

	// close conn
	conn.Close()
}
