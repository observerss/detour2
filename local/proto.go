package local

import "net"

type Request struct {
	Network string
	Address string
}

type Proto interface {
	Get(conn net.Conn) (*Request, error)          // Get Request from Client
	Ack(conn net.Conn, ok bool, msg string) error // Ack Request
}
