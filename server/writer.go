package server

import (
	"errors"

	"github.com/gorilla/websocket"
	"github.com/observerss/detour2/common"
)

func (s *Server) NewWebsocketWriter(conn *websocket.Conn) *common.FairMessageWriter {
	return common.NewFairMessageWriter(func(msg *common.Message) error {
		data, err := s.Packer.Pack(msg)
		if err == nil {
			err = conn.WriteMessage(websocket.BinaryMessage, data)
		}
		if s.Metrics != nil {
			if err != nil {
				s.Metrics.WebSocketWriteErrors.Inc()
			} else {
				s.Metrics.MessagesOutTotal.Inc()
				s.Metrics.PayloadBytesOutTotal.Add(int64(len(msg.Data)))
			}
		}
		return err
	}, common.DefaultMessageQueueLimit)
}

func (s *Server) writeWebsocket(writer *common.FairMessageWriter, msg *common.Message) error {
	if writer == nil {
		return errors.New("websocket writer is not connected")
	}
	err := writer.Write(msg)
	if s.Metrics != nil && errors.Is(err, common.ErrMessageQueueFull) {
		s.Metrics.QueueFullTotal.Inc()
	}
	return err
}
