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

type DeployConfig struct {
	AccessKeyId     string `json:"accessKeyId"`
	AccessKeySecret string `json:"accessKeySecret"`
	AccountId       string `json:"accountId"`
	Region          string `json:"region" example:"cn-hongkong"`
	ServiceName     string `json:"serviceName" example:"api2"`
	FunctionName    string `json:"functionName" example:"dt2"`
	TriggerName     string `json:"triggerName" example:"ws2"`
	Password        string `json:"password" example:"pass123"`
	Image           string `json:"image" example:"registry-vpc.cn-hongkong.aliyuncs.com/hjcrocks/detour2:0.2.5"`
	PublicPort      int    `json:"publicPort" example:"3810"`
	Remove          bool   `json:"remove" example:"false"`
}
