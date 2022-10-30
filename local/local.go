package local

import (
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var proto = "socks5"

type Local struct {
	Address    string
	RemoteUrls []string
}

func NewLocal(address string, remoteUrls []string) *Local {
	return &Local{RemoteUrls: remoteUrls}
}

func (l *Local) Run() {
	listener, err := net.Listen("tcp", l.Address)
	client := NewClient(l.RemoteUrls)
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
	for {
		go func() {
			conn, err := listener.Accept()
			if err != nil {
				log.Println("accept error:", err)
				connch <- nil
			} else {
				connch <- conn
			}
		}()

		select {
		case <-quit:
			wg.Wait()
			log.Println("byebye")
			return
		case conn := <-connch:
			if conn != nil {
				handler, err := NewHandler(proto, conn, client, quit)
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
