package common

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestFairMessageWriterRoundRobinByCID(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	written := []string{}

	writer := NewFairMessageWriter(func(msg *Message) error {
		mu.Lock()
		written = append(written, msg.Cid)
		if len(written) == 1 {
			close(started)
		}
		mu.Unlock()
		<-release
		return nil
	}, 8)
	defer writer.Close()

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- writer.Write(&Message{Cid: "a", Data: []byte("a1")})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("writer did not start first message")
	}

	results := make([]chan error, 0, 4)
	for _, item := range []struct {
		cid       string
		queueSize int
	}{
		{cid: "a", queueSize: 1},
		{cid: "a", queueSize: 2},
		{cid: "b", queueSize: 1},
		{cid: "b", queueSize: 2},
	} {
		result := make(chan error, 1)
		results = append(results, result)
		go func(cid string) {
			result <- writer.Write(&Message{Cid: cid, Data: []byte(cid)})
		}(item.cid)
		waitQueueLen(t, writer, item.cid, item.queueSize)
	}

	for i := 0; i < 5; i++ {
		release <- struct{}{}
	}
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	}

	mu.Lock()
	got := append([]string{}, written...)
	mu.Unlock()
	if !reflect.DeepEqual(got, []string{"a", "a", "b", "a", "b"}) {
		t.Fatalf("unexpected fair order: %+v", got)
	}
}

func TestFairMessageWriterQueueFull(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	writer := NewFairMessageWriter(func(msg *Message) error {
		startOnce.Do(func() { close(started) })
		<-release
		return nil
	}, 1)
	defer writer.Close()

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- writer.Write(&Message{Cid: "a"})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("writer did not start first message")
	}

	queued := make(chan error, 1)
	go func() {
		queued <- writer.Write(&Message{Cid: "a"})
	}()
	waitQueueLen(t, writer, "a", 1)
	if err := writer.Write(&Message{Cid: "a"}); !errors.Is(err, ErrMessageQueueFull) {
		t.Fatalf("expected queue full, got %v", err)
	}

	release <- struct{}{}
	release <- struct{}{}
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-queued; err != nil {
		t.Fatal(err)
	}
}

func TestFairMessageWriterBusyCIDDoesNotStarveOthers(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	var mu sync.Mutex
	written := []string{}

	writer := NewFairMessageWriter(func(msg *Message) error {
		startOnce.Do(func() { close(started) })
		mu.Lock()
		written = append(written, msg.Cid)
		mu.Unlock()
		<-release
		return nil
	}, 16)
	defer writer.Close()

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- writer.Write(&Message{Cid: "busy"})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("writer did not start first message")
	}

	busyResults := make([]chan error, 0, 8)
	for idx := 1; idx <= 8; idx++ {
		result := make(chan error, 1)
		busyResults = append(busyResults, result)
		go func() {
			result <- writer.Write(&Message{Cid: "busy"})
		}()
		waitQueueLen(t, writer, "busy", idx)
	}
	fastResult := make(chan error, 1)
	go func() {
		fastResult <- writer.Write(&Message{Cid: "fast"})
	}()
	waitQueueLen(t, writer, "fast", 1)

	for idx := 0; idx < 3; idx++ {
		release <- struct{}{}
	}
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-busyResults[0]; err != nil {
		t.Fatal(err)
	}
	if err := <-fastResult; err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	got := append([]string{}, written...)
	mu.Unlock()
	if len(got) < 3 || got[2] != "fast" {
		t.Fatalf("fast cid was starved behind busy cid: %+v", got)
	}

	for idx := 1; idx < len(busyResults); idx++ {
		release <- struct{}{}
		if err := <-busyResults[idx]; err != nil {
			t.Fatal(err)
		}
	}
}

func waitQueueLen(t *testing.T, writer *FairMessageWriter, cid string, size int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	key := messageQueueKey(&Message{Cid: cid})
	for time.Now().Before(deadline) {
		writer.mu.Lock()
		got := len(writer.queues[key])
		writer.mu.Unlock()
		if got == size {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("queue %s did not reach size %d", cid, size)
}
