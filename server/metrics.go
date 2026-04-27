package server

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/observerss/detour2/common"
	"github.com/observerss/detour2/logger"
)

type ServerMetricsSnapshot struct {
	Role        string                        `json:"role"`
	Listen      string                        `json:"listen"`
	DNSServers  []string                      `json:"dnsServers,omitempty"`
	Runtime     common.RuntimeMetricsSnapshot `json:"runtime"`
	Connections ServerConnectionSnapshot      `json:"connections"`
	RelayPool   ServerRelayPoolSnapshot       `json:"relayPool,omitempty"`
}

type ServerConnectionSnapshot struct {
	Active  int                      `json:"active"`
	ByWID   map[string]int           `json:"byWid"`
	Writers ServerWriterPoolSnapshot `json:"writers"`
}

type ServerWriterPoolSnapshot struct {
	Total         int `json:"total"`
	Closed        int `json:"closed"`
	QueueKeys     int `json:"queueKeys"`
	QueueMessages int `json:"queueMessages"`
}

type ServerRelayPoolSnapshot struct {
	Total       int                   `json:"total"`
	Connected   int                   `json:"connected"`
	ActiveTotal int64                 `json:"activeTotal"`
	MaxActive   int64                 `json:"maxActive"`
	Items       []ServerRelaySnapshot `json:"items"`
}

type ServerRelaySnapshot struct {
	Key       string                           `json:"key"`
	URL       string                           `json:"url"`
	WID       string                           `json:"wid"`
	Connected bool                             `json:"connected"`
	Active    int64                            `json:"active"`
	Writer    common.FairMessageWriterSnapshot `json:"writer"`
}

func (s *Server) MetricsSnapshot() ServerMetricsSnapshot {
	connections := ServerConnectionSnapshot{ByWID: map[string]int{}}
	seenWriters := map[*common.FairMessageWriter]bool{}
	s.Conns.Range(func(key, value any) bool {
		conn := value.(*Conn)
		connections.Active++
		connections.ByWID[conn.Wid]++
		_, connWriter := conn.Transport()
		if connWriter != nil && !seenWriters[connWriter] {
			seenWriters[connWriter] = true
			writer := connWriter.Snapshot()
			connections.Writers.Total++
			if writer.Closed {
				connections.Writers.Closed++
			}
			connections.Writers.QueueKeys += writer.QueueKeys
			connections.Writers.QueueMessages += writer.QueueMessages
		}
		return true
	})

	relayPool := ServerRelayPoolSnapshot{Total: len(s.RelayClients)}
	if len(s.RelayClients) > 0 {
		items := make([]ServerRelaySnapshot, 0, len(s.RelayClients))
		for key, relay := range s.RelayClients {
			relay.RWLock.RLock()
			connected := relay.Connected
			relay.RWLock.RUnlock()

			relay.WriteLock.Lock()
			writerSnapshot := relay.Writer.Snapshot()
			relay.WriteLock.Unlock()

			item := ServerRelaySnapshot{
				Key:       key,
				URL:       relay.Url,
				WID:       relay.Wid,
				Connected: connected,
				Active:    relay.ActiveCount(),
				Writer:    writerSnapshot,
			}

			if item.Connected {
				relayPool.Connected++
			}
			relayPool.ActiveTotal += item.Active
			if item.Active > relayPool.MaxActive {
				relayPool.MaxActive = item.Active
			}
			items = append(items, item)
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].Key < items[j].Key
		})
		relayPool.Items = items
	}

	role := "server"
	if s.HasNextRelay() {
		role = "relay"
	}

	return ServerMetricsSnapshot{
		Role:        role,
		Listen:      s.Address,
		DNSServers:  append([]string{}, s.DNSServers...),
		Runtime:     s.Metrics.Snapshot(),
		Connections: connections,
		RelayPool:   relayPool,
	}
}

func (s *Server) StartMetricsServer() {
	if s.MetricsListen == "" {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/metrics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(s.MetricsSnapshot()); err != nil {
			logger.Error.Println("metrics, encode error", err)
		}
	})

	go func() {
		logger.Info.Println("Metrics listening at http://" + s.MetricsListen + "/debug/metrics")
		if err := http.ListenAndServe(s.MetricsListen, mux); err != nil && err != http.ErrServerClosed {
			logger.Error.Println("metrics, listen error", err)
		}
	}()
}
