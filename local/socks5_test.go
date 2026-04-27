package local

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

func runSocks5Request(t *testing.T, request []byte) (*Request, error) {
	t.Helper()

	client, server := net.Pipe()
	t.Cleanup(func() {
		client.Close()
		server.Close()
	})

	deadline := time.Now().Add(time.Second)
	if err := client.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	if err := server.SetDeadline(deadline); err != nil {
		t.Fatal(err)
	}

	type result struct {
		req *Request
		err error
	}
	resultCh := make(chan result, 1)
	go func() {
		req, err := (&Socks5Proto{}).Get(server)
		resultCh <- result{req: req, err: err}
	}()

	if _, err := client.Write([]byte{SOCKS5_VERSION, 1, METHOD_NOAUTH}); err != nil {
		t.Fatal(err)
	}

	ack := make([]byte, len(ACCEPT_METHOD_AUTH))
	if _, err := io.ReadFull(client, ack); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ack, ACCEPT_METHOD_AUTH) {
		t.Fatalf("unexpected auth ack: %v", ack)
	}

	if _, err := client.Write(request); err != nil {
		t.Fatal(err)
	}

	select {
	case result := <-resultCh:
		return result.req, result.err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for socks5 parser")
		return nil, nil
	}
}

func TestSocks5ProtoParsesShortDomain(t *testing.T) {
	req, err := runSocks5Request(t, []byte{
		SOCKS5_VERSION, SOCKS5_CONNECT, 0, ADDR_DOMAIN,
		1, 'a', 0, 80,
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.Network != "tcp" || req.Address != "a:80" {
		t.Fatalf("unexpected request: %+v", req)
	}
}

func TestSocks5ProtoRejectsTruncatedDomain(t *testing.T) {
	_, err := runSocks5Request(t, []byte{
		SOCKS5_VERSION, SOCKS5_CONNECT, 0, ADDR_DOMAIN,
		10, 'a', 'b',
	})
	if err == nil {
		t.Fatal("expected truncated domain request error")
	}
}
