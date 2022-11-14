package local

import (
	"io"
	"net"
)

type Request struct {
	Network string
	Address string
	More    bool
	Reader  io.Reader
}

type Proto interface {
	Get(conn net.Conn) (*Request, error)                        // Get Request from Client
	Ack(conn net.Conn, ok bool, msg string, req *Request) error // Ack Request
}
