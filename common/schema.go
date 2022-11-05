package common

type CMD int

const (
	CONNECT CMD = iota
	DATA
	CLOSE
	SWITCH
)

type Message struct {
	Cmd     CMD
	Wid     string
	Cid     string
	Ok      bool
	Msg     string
	Network string
	Address string
	Data    []byte
}

type Package struct {
	Key     []byte
	Padding int
	Message *Message
}

type LocalConfig struct {
	Listen   string `json:"listen" example:"tcp://0.0.0.0:3810"`
	Remotes  string `json:"remotes" example:"ws://127.0.0.1:3811/ws,ws://127.0.0.1:3811/ws"`
	Password string `json:"password" example:"pass123"`
	Proto    string `json:"proto" example:"socks5"`
}

type ServerConfig struct {
	Listen   string `json:"listen" example:"tcp://0.0.0.0:3811"`
	Password string `json:"password" example:"pass123"`
}
