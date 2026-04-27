package common

import (
	"sync/atomic"
	"time"
)

type Counter struct {
	value atomic.Int64
}

func (c *Counter) Add(delta int64) {
	c.value.Add(delta)
}

func (c *Counter) Inc() {
	c.Add(1)
}

func (c *Counter) Dec() {
	c.Add(-1)
}

func (c *Counter) Load() int64 {
	return c.value.Load()
}

type RuntimeMetrics struct {
	StartedAt time.Time

	ClientConnectionsTotal  Counter
	ClientConnectionsClosed Counter
	WebSocketConnectsTotal  Counter
	WebSocketActive         Counter
	WebSocketReadErrors     Counter
	WebSocketWriteErrors    Counter
	ConnectAttemptsTotal    Counter
	ConnectFailuresTotal    Counter
	RelayConnectFailures    Counter
	QueueTimeoutsTotal      Counter
	QueueFullTotal          Counter
	MessagesInTotal         Counter
	MessagesOutTotal        Counter
	PayloadBytesInTotal     Counter
	PayloadBytesOutTotal    Counter
}

type RuntimeMetricsSnapshot struct {
	StartedAt               string `json:"startedAt"`
	UptimeSeconds           int64  `json:"uptimeSeconds"`
	ClientConnectionsTotal  int64  `json:"clientConnectionsTotal"`
	ClientConnectionsClosed int64  `json:"clientConnectionsClosed"`
	WebSocketConnectsTotal  int64  `json:"webSocketConnectsTotal"`
	WebSocketActive         int64  `json:"webSocketActive"`
	WebSocketReadErrors     int64  `json:"webSocketReadErrors"`
	WebSocketWriteErrors    int64  `json:"webSocketWriteErrors"`
	ConnectAttemptsTotal    int64  `json:"connectAttemptsTotal"`
	ConnectFailuresTotal    int64  `json:"connectFailuresTotal"`
	RelayConnectFailures    int64  `json:"relayConnectFailures"`
	QueueTimeoutsTotal      int64  `json:"queueTimeoutsTotal"`
	QueueFullTotal          int64  `json:"queueFullTotal"`
	MessagesInTotal         int64  `json:"messagesInTotal"`
	MessagesOutTotal        int64  `json:"messagesOutTotal"`
	PayloadBytesInTotal     int64  `json:"payloadBytesInTotal"`
	PayloadBytesOutTotal    int64  `json:"payloadBytesOutTotal"`
}

func NewRuntimeMetrics() *RuntimeMetrics {
	return &RuntimeMetrics{StartedAt: time.Now()}
}

func (m *RuntimeMetrics) Snapshot() RuntimeMetricsSnapshot {
	if m == nil {
		return RuntimeMetricsSnapshot{}
	}
	return RuntimeMetricsSnapshot{
		StartedAt:               m.StartedAt.Format(time.RFC3339),
		UptimeSeconds:           int64(time.Since(m.StartedAt).Seconds()),
		ClientConnectionsTotal:  m.ClientConnectionsTotal.Load(),
		ClientConnectionsClosed: m.ClientConnectionsClosed.Load(),
		WebSocketConnectsTotal:  m.WebSocketConnectsTotal.Load(),
		WebSocketActive:         m.WebSocketActive.Load(),
		WebSocketReadErrors:     m.WebSocketReadErrors.Load(),
		WebSocketWriteErrors:    m.WebSocketWriteErrors.Load(),
		ConnectAttemptsTotal:    m.ConnectAttemptsTotal.Load(),
		ConnectFailuresTotal:    m.ConnectFailuresTotal.Load(),
		RelayConnectFailures:    m.RelayConnectFailures.Load(),
		QueueTimeoutsTotal:      m.QueueTimeoutsTotal.Load(),
		QueueFullTotal:          m.QueueFullTotal.Load(),
		MessagesInTotal:         m.MessagesInTotal.Load(),
		MessagesOutTotal:        m.MessagesOutTotal.Load(),
		PayloadBytesInTotal:     m.PayloadBytesInTotal.Load(),
		PayloadBytesOutTotal:    m.PayloadBytesOutTotal.Load(),
	}
}
