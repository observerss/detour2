package local

import "net"

type ShadowProtocol struct {
	Conn net.Conn
}

func (s *ShadowProtocol) Init() *Remote {
	// TODO: implement this
	return nil
}

func (s *ShadowProtocol) Bind(err error) error {
	return nil
}
