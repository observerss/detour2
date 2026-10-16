package local

import (
	"net"
	"testing"
	"time"

	"github.com/observerss/detour2/common"
)

func TestStopLocalBeforeRunReleasesPort(t *testing.T) {
	silenceLogs(t)

	addr := unusedTCPAddress(t)
	l := NewLocal(&common.LocalConfig{
		Listen:   "tcp://" + addr,
		Remotes:  "ws://127.0.0.1:1/ws",
		Password: integrationTestPassword,
		Proto:    PROTO_SOCKS5,
	})
	l.StopLocal()

	done := make(chan error, 1)
	go func() { done <- l.RunLocal() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunLocal after StopLocal: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunLocal kept running after StopLocal")
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("port still held after stop: %v", err)
	}
	listener.Close()
}
