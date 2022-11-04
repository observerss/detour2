package local

import "strings"

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

func (l *Local) RunLocal() {

}
