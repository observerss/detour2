package local

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
)

type HTTPProto struct{}

func (p *HTTPProto) Get(conn net.Conn) (*Request, error) {
	reader := bufio.NewReader(conn)
	r, err := http.ReadRequest(reader)
	if err != nil {
		return nil, err
	}
	host := r.Host
	if !strings.Contains(r.Host, ":") {
		host += ":80"
	}
	req := &Request{
		Network: "tcp",
		Address: host,
	}
	if r.Method == "CONNECT" {
		return req, nil
	}

	r.RequestURI = ""
	req.More = true
	reader2, writer := io.Pipe()
	go func() {
		r.Write(writer)
	}()
	req.Reader = reader2
	return req, nil
}

func (p *HTTPProto) Ack(conn net.Conn, ok bool, msg string, req *Request) error {
	// http proxy does not need to ack
	if !req.More {
		// CONNECT need reply 2xx successful message
		conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
	}
	return nil
}
