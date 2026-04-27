package local

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/observerss/detour2/common"
	"github.com/observerss/detour2/logger"
	"github.com/observerss/detour2/server"
)

const integrationTestPassword = "pass123"

func TestProxyStackSocks5ConnectEcho(t *testing.T) {
	silenceLogs(t)

	targetAddr := startTCPEchoServer(t)
	proxyAddr := startProxyStack(t, PROTO_SOCKS5)

	conn := dialProxy(t, proxyAddr)
	defer conn.Close()

	writeSocks5Connect(t, conn, targetAddr)
	assertSocks5Reply(t, conn, true)

	payload := []byte("socks5 integration payload")
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("unexpected echo payload: %q", got)
	}
}

func TestProxyStackHTTPConnectEcho(t *testing.T) {
	silenceLogs(t)

	targetAddr := startTCPEchoServer(t)
	proxyAddr := startProxyStack(t, PROTO_HTTP)

	conn := dialProxy(t, proxyAddr)
	defer conn.Close()
	reader := bufio.NewReader(conn)

	fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", targetAddr, targetAddr)
	resp, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected CONNECT status: %s", resp.Status)
	}

	payload := []byte("http connect integration payload")
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(reader, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("unexpected echo payload: %q", got)
	}
}

func TestProxyStackSocks5ConnectFailure(t *testing.T) {
	silenceLogs(t)

	proxyAddr := startProxyStack(t, PROTO_SOCKS5)
	conn := dialProxy(t, proxyAddr)
	defer conn.Close()

	writeSocks5Connect(t, conn, unusedTCPAddress(t))
	assertSocks5Reply(t, conn, false)
}

func TestProxyStackSocks5MultiHopRelayEcho(t *testing.T) {
	silenceLogs(t)

	targetAddr := startTCPEchoServer(t)
	exitRelayURL := startRelayServer(t, "")
	middleRelay, middleRelayURL := startRelayServerWithServer(t, exitRelayURL)
	proxyAddr := startProxyToRemote(t, PROTO_SOCKS5, middleRelayURL)

	conn := dialProxy(t, proxyAddr)
	defer conn.Close()

	writeSocks5Connect(t, conn, targetAddr)
	assertSocks5Reply(t, conn, true)

	payload := []byte("multi hop relay payload")
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("unexpected echo payload: %q", got)
	}
	conn.Close()
	waitForRelayIdle(t, middleRelay)
}

func TestProxyStackSocks5MultiHopRelayReconnects(t *testing.T) {
	silenceLogs(t)

	targetAddr := startTCPEchoServer(t)
	_, exitRelayURL := startRelayServerWithServer(t, "")
	middleRelay, middleRelayURL := startRelayServerWithServer(t, exitRelayURL)
	proxyAddr := startProxyToRemote(t, PROTO_SOCKS5, middleRelayURL)

	assertSocks5Echo(t, proxyAddr, targetAddr, []byte("before relay reconnect"))
	disconnectRelayClient(t, firstRelayClient(t, middleRelay))
	assertSocks5Echo(t, proxyAddr, targetAddr, []byte("after relay reconnect"))
}

func startProxyStack(t *testing.T, proto string) string {
	t.Helper()
	return startProxyToRemote(t, proto, startRelayServer(t, ""))
}

func startRelayServer(t *testing.T, nextRemotes string) string {
	t.Helper()
	_, url := startRelayServerWithServer(t, nextRemotes)
	return url
}

func startRelayServerWithServer(t *testing.T, nextRemotes string) (*server.Server, string) {
	t.Helper()

	remote := server.NewServer(&common.ServerConfig{
		Listen:   "tcp://127.0.0.1:0",
		Remotes:  nextRemotes,
		Password: integrationTestPassword,
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", remote.HandleWebsocket)
	wsServer := httptest.NewServer(mux)
	t.Cleanup(wsServer.Close)
	return remote, "ws" + strings.TrimPrefix(wsServer.URL, "http") + "/ws"
}

func startProxyToRemote(t *testing.T, proto string, remoteURL string) string {
	t.Helper()

	proxy := NewLocal(&common.LocalConfig{
		Listen:   "tcp://127.0.0.1:0",
		Remotes:  remoteURL,
		Password: integrationTestPassword,
		Proto:    proto,
	})
	listener, err := net.Listen(proxy.Network, proxy.Address)
	if err != nil {
		t.Fatal(err)
	}
	proxy.Listener = listener

	for _, wsconn := range proxy.WSConns {
		go wsconn.WebsocketPuller()
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go proxy.HandleConn(conn)
		}
	}()
	t.Cleanup(proxy.StopLocal)

	return listener.Addr().String()
}

func startTCPEchoServer(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				io.Copy(conn, conn)
			}(conn)
		}
	}()
	t.Cleanup(func() {
		listener.Close()
	})

	return listener.Addr().String()
}

func unusedTCPAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func firstRelayClient(t *testing.T, remote *server.Server) *server.RelayClient {
	t.Helper()

	for _, relay := range remote.RelayClients {
		return relay
	}
	t.Fatal("server has no relay clients")
	return nil
}

func disconnectRelayClient(t *testing.T, relay *server.RelayClient) {
	t.Helper()

	relay.WriteLock.Lock()
	if relay.WSConn != nil {
		if err := relay.WSConn.Close(); err != nil {
			relay.WriteLock.Unlock()
			t.Fatal(err)
		}
		relay.WSConn = nil
	}
	relay.WriteLock.Unlock()
	relay.SetConnected(false)
}

func waitForRelayIdle(t *testing.T, remote *server.Server) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := remote.MetricsSnapshot()
		if snapshot.Connections.Active == 0 && snapshot.RelayPool.ActiveTotal == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	snapshot := remote.MetricsSnapshot()
	t.Fatalf("relay did not become idle: connections=%+v relayPool=%+v", snapshot.Connections, snapshot.RelayPool)
}

func dialProxy(t *testing.T, addr string) net.Conn {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	return conn
}

func assertSocks5Echo(t *testing.T, proxyAddr string, targetAddr string, payload []byte) {
	t.Helper()

	conn := dialProxy(t, proxyAddr)
	defer conn.Close()

	writeSocks5Connect(t, conn, targetAddr)
	assertSocks5Reply(t, conn, true)

	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("unexpected echo payload: %q", got)
	}
}

func writeSocks5Connect(t *testing.T, conn net.Conn, targetAddr string) {
	t.Helper()

	if _, err := conn.Write([]byte{SOCKS5_VERSION, 1, METHOD_NOAUTH}); err != nil {
		t.Fatal(err)
	}
	authReply := make([]byte, len(ACCEPT_METHOD_AUTH))
	if _, err := io.ReadFull(conn, authReply); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(authReply, ACCEPT_METHOD_AUTH) {
		t.Fatalf("unexpected auth reply: %v", authReply)
	}

	host, portValue, err := net.SplitHostPort(targetAddr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portValue)
	if err != nil {
		t.Fatal(err)
	}
	if len(host) > 255 {
		t.Fatalf("host is too long for socks5 domain request: %q", host)
	}

	request := []byte{SOCKS5_VERSION, SOCKS5_CONNECT, 0, ADDR_DOMAIN, byte(len(host))}
	request = append(request, host...)
	request = binary.BigEndian.AppendUint16(request, uint16(port))
	if _, err := conn.Write(request); err != nil {
		t.Fatal(err)
	}
}

func assertSocks5Reply(t *testing.T, conn net.Conn, ok bool) {
	t.Helper()

	reply := make([]byte, len(CMD_OK))
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatal(err)
	}
	want := CMD_OK
	if !ok {
		want = CMD_FAILED
	}
	if !bytes.Equal(reply, want) {
		t.Fatalf("unexpected socks5 reply: got %v want %v", reply, want)
	}
}

func silenceLogs(t *testing.T) {
	t.Helper()

	debugOut := logger.Debug.Writer()
	infoOut := logger.Info.Writer()
	warnOut := logger.Warn.Writer()
	errorOut := logger.Error.Writer()
	logger.Debug.SetOutput(io.Discard)
	logger.Info.SetOutput(io.Discard)
	logger.Warn.SetOutput(io.Discard)
	logger.Error.SetOutput(io.Discard)
	t.Cleanup(func() {
		logger.Debug.SetOutput(debugOut)
		logger.Info.SetOutput(infoOut)
		logger.Warn.SetOutput(warnOut)
		logger.Error.SetOutput(errorOut)
	})
}
