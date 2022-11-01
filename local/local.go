package local

import (
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

type Local struct {
	Network    string
	Address    string
	Proto      string
	Password   string
	RemoteUrls []string
}

func NewLocal(listen string, remote string, proto string, password string) *Local {
	vals := strings.Split(listen, "://")
	network := vals[0]
	address := vals[1]
	remoteUrls := strings.Split(remote, ",")
	return &Local{RemoteUrls: remoteUrls, Network: network, Address: address, Password: password, Proto: proto}
}

func (l *Local) Run() {
	log.Println("Listening on", l.Address)
	listener, err := net.Listen(l.Network, l.Address)
	client := NewClient(l.RemoteUrls, l.Password)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	// graceful shutdown
	quit := make(chan struct{})
	go func() {
		sigch := make(chan os.Signal, 1)
		signal.Notify(sigch, syscall.SIGINT, syscall.SIGTERM)
		<-sigch
		close(quit)
	}()

	// server loop
	wg := sync.WaitGroup{}
	connch := make(chan net.Conn)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Println("accept error:", err)
				connch <- nil
			} else {
				connch <- conn
			}
		}
	}()

	for {
		select {
		case <-quit:
			listener.Close()
			wg.Wait()
			log.Println("byebye")
			return
		case conn := <-connch:
			if conn != nil {
				handler, err := NewHandler(l.Proto, conn, client, quit)
				if err != nil {
					log.Println("create handler error:", err)
				} else {
					wg.Add(1)
					go func() {
						handler.HandleConn()
						wg.Done()
					}()
				}
			}
		}
	}
}
