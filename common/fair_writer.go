package common

import (
	"errors"
	"sync"
)

const DefaultMessageQueueLimit = 64

var (
	ErrMessageWriterClosed = errors.New("message writer is closed")
	ErrMessageQueueFull    = errors.New("message queue is full")
)

type MessageWriteFunc func(*Message) error

type FairMessageWriter struct {
	mu       sync.Mutex
	cond     *sync.Cond
	queues   map[string][]*queuedMessage
	order    []string
	closed   bool
	lastErr  error
	write    MessageWriteFunc
	maxQueue int
	done     chan struct{}
	once     sync.Once
}

type FairMessageWriterSnapshot struct {
	Closed         bool `json:"closed"`
	QueueKeys      int  `json:"queueKeys"`
	QueueMessages  int  `json:"queueMessages"`
	MaxQueuePerCID int  `json:"maxQueuePerCid"`
}

type queuedMessage struct {
	msg    *Message
	result chan error
}

func NewFairMessageWriter(write MessageWriteFunc, maxQueuePerCID int) *FairMessageWriter {
	if maxQueuePerCID < 1 {
		maxQueuePerCID = DefaultMessageQueueLimit
	}
	writer := &FairMessageWriter{
		queues:   make(map[string][]*queuedMessage),
		write:    write,
		maxQueue: maxQueuePerCID,
		done:     make(chan struct{}),
	}
	writer.cond = sync.NewCond(&writer.mu)
	go writer.run()
	return writer
}

func (w *FairMessageWriter) Write(msg *Message) error {
	if w == nil {
		return ErrMessageWriterClosed
	}
	item := &queuedMessage{
		msg:    CloneMessage(msg),
		result: make(chan error, 1),
	}
	key := messageQueueKey(msg)

	w.mu.Lock()
	if w.closed {
		err := w.closeErrLocked()
		w.mu.Unlock()
		return err
	}
	queue := w.queues[key]
	if len(queue) >= w.maxQueue {
		w.mu.Unlock()
		return ErrMessageQueueFull
	}
	if len(queue) == 0 {
		w.order = append(w.order, key)
	}
	w.queues[key] = append(queue, item)
	w.cond.Signal()
	w.mu.Unlock()

	return <-item.result
}

func (w *FairMessageWriter) Close() {
	w.closeWithError(ErrMessageWriterClosed)
}

func (w *FairMessageWriter) Done() <-chan struct{} {
	if w == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return w.done
}

func (w *FairMessageWriter) Snapshot() FairMessageWriterSnapshot {
	if w == nil {
		return FairMessageWriterSnapshot{Closed: true}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	snapshot := FairMessageWriterSnapshot{
		Closed:         w.closed,
		QueueKeys:      len(w.queues),
		MaxQueuePerCID: w.maxQueue,
	}
	for _, queue := range w.queues {
		snapshot.QueueMessages += len(queue)
	}
	return snapshot
}

func (w *FairMessageWriter) run() {
	defer close(w.done)
	for {
		item, ok := w.next()
		if !ok {
			return
		}
		err := w.write(item.msg)
		item.result <- err
		if err != nil {
			w.closeWithError(err)
			return
		}
	}
}

func (w *FairMessageWriter) next() (*queuedMessage, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for len(w.order) == 0 && !w.closed {
		w.cond.Wait()
	}
	if len(w.order) == 0 && w.closed {
		return nil, false
	}

	key := w.order[0]
	queue := w.queues[key]
	item := queue[0]
	queue = queue[1:]
	if len(queue) == 0 {
		delete(w.queues, key)
		w.order = w.order[1:]
	} else {
		w.queues[key] = queue
		copy(w.order, w.order[1:])
		w.order[len(w.order)-1] = key
	}
	return item, true
}

func (w *FairMessageWriter) closeWithError(err error) {
	w.once.Do(func() {
		w.mu.Lock()
		w.closed = true
		w.lastErr = err
		for key, queue := range w.queues {
			for _, item := range queue {
				item.result <- err
			}
			delete(w.queues, key)
		}
		w.order = nil
		w.cond.Broadcast()
		w.mu.Unlock()
	})
}

func (w *FairMessageWriter) closeErrLocked() error {
	if w.lastErr != nil {
		return w.lastErr
	}
	return ErrMessageWriterClosed
}

func CloneMessage(msg *Message) *Message {
	if msg == nil {
		return nil
	}
	clone := *msg
	if msg.Data != nil {
		clone.Data = append([]byte{}, msg.Data...)
	}
	return &clone
}

func messageQueueKey(msg *Message) string {
	if msg == nil || msg.Cid == "" {
		return "__control__"
	}
	return msg.Cid
}
