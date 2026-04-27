package local

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/observerss/detour2/common"
	"github.com/observerss/detour2/logger"
)

type LocalMetricsSnapshot struct {
	Role          string                        `json:"role"`
	Listen        string                        `json:"listen"`
	Proto         string                        `json:"proto"`
	Runtime       common.RuntimeMetricsSnapshot `json:"runtime"`
	Connections   LocalConnectionSnapshot       `json:"connections"`
	WebSocketPool LocalWebSocketPoolSnapshot    `json:"webSocketPool"`
}

type LocalConnectionSnapshot struct {
	Active int `json:"active"`
}

type LocalWebSocketPoolSnapshot struct {
	Total       int                      `json:"total"`
	Connected   int                      `json:"connected"`
	Connectable int                      `json:"connectable"`
	ActiveTotal int64                    `json:"activeTotal"`
	MaxActive   int64                    `json:"maxActive"`
	Items       []LocalWebSocketSnapshot `json:"items"`
}

type LocalWebSocketSnapshot struct {
	Key        string                           `json:"key"`
	URL        string                           `json:"url"`
	WID        string                           `json:"wid"`
	Connected  bool                             `json:"connected"`
	CanConnect bool                             `json:"canConnect"`
	Active     int64                            `json:"active"`
	Writer     common.FairMessageWriterSnapshot `json:"writer"`
}

func (l *Local) MetricsSnapshot() LocalMetricsSnapshot {
	activeConnections := 0
	l.Conns.Range(func(key, value any) bool {
		activeConnections++
		return true
	})

	items := make([]LocalWebSocketSnapshot, 0, len(l.WSConns))
	pool := LocalWebSocketPoolSnapshot{Total: len(l.WSConns)}
	for key, wsconn := range l.WSConns {
		wsconn.RWLock.RLock()
		connected := wsconn.Connected
		canConnect := wsconn.CanConnect
		wsconn.RWLock.RUnlock()

		wsconn.WriteLock.Lock()
		writerSnapshot := wsconn.Writer.Snapshot()
		wsconn.WriteLock.Unlock()

		item := LocalWebSocketSnapshot{
			Key:        key,
			URL:        wsconn.Url,
			WID:        wsconn.Wid,
			Connected:  connected,
			CanConnect: canConnect,
			Active:     wsconn.ActiveCount(),
			Writer:     writerSnapshot,
		}

		if item.Connected {
			pool.Connected++
		}
		if item.CanConnect || item.Connected {
			pool.Connectable++
		}
		pool.ActiveTotal += item.Active
		if item.Active > pool.MaxActive {
			pool.MaxActive = item.Active
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Key < items[j].Key
	})
	pool.Items = items

	return LocalMetricsSnapshot{
		Role:          "local",
		Listen:        l.Network + "://" + l.Address,
		Proto:         protoName(l.Proto),
		Runtime:       l.Metrics.Snapshot(),
		Connections:   LocalConnectionSnapshot{Active: activeConnections},
		WebSocketPool: pool,
	}
}

func (l *Local) StartMetricsServer() {
	if l.MetricsListen == "" {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/metrics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(l.MetricsSnapshot()); err != nil {
			logger.Error.Println("metrics, encode error", err)
		}
	})

	server := &http.Server{Addr: l.MetricsListen, Handler: mux}
	go func() {
		<-l.DoneChan()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	go func() {
		logger.Info.Println("Metrics listening at http://" + l.MetricsListen + "/debug/metrics")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error.Println("metrics, listen error", err)
		}
	}()
}

func protoName(proto Proto) string {
	switch proto.(type) {
	case *Socks5Proto:
		return PROTO_SOCKS5
	case *HTTPProto:
		return PROTO_HTTP
	default:
		return "unknown"
	}
}
