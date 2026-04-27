package server

import (
	"sync"
	"testing"
)

func TestWSCounterConcurrentAccess(t *testing.T) {
	server := &Server{WSCounter: make(map[string]int)}
	var wg sync.WaitGroup

	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				server.incrementWSCounter("wid")
				server.decrementWSCounter("wid")
			}
		}()
	}
	wg.Wait()

	if got := server.WSCounter["wid"]; got != 0 {
		t.Fatalf("unexpected websocket counter: %d", got)
	}
}
