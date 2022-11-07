package common

import (
	"log"
	"testing"
)

func Test_PACKER(t *testing.T) {
	msg := Message{Cmd: CONNECT, Network: "network", Address: "address", Cid: "cicicici", Wid: "didi", Data: []byte{1, 2, 3, 4, 244, 233, 222, 211}}
	p := Packer{Password: "pass123"}
	data, _ := p.Pack(&msg)
	log.Println("packed", data)
	msg2, _ := p.Unpack(data)
	log.Println("unpacked", msg2)
	if msg.Cmd != msg2.Cmd || msg.Network != msg2.Network || msg.Address != msg2.Address || msg.Cid != msg2.Cid || msg.Wid != msg2.Wid || string(msg.Data) != string(msg2.Data) {
		t.Error("msg and msg2 does not match", msg, msg2)
	}
}
