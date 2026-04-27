package common

import (
	"bytes"
	"encoding/gob"
	"log"
	"testing"
)

func Test_PACKER(t *testing.T) {
	msg := Message{Cmd: CONNECT, Network: "network", Address: "address", Cid: "cicicici", Wid: "didi", Data: []byte{1, 2, 3, 4, 244, 233, 222, 211}}
	p := Packer{Password: "pass123"}
	data, err := p.Pack(&msg)
	if err != nil {
		t.Fatal(err)
	}
	log.Println("packed", data)
	msg2, err := p.Unpack(data)
	if err != nil {
		t.Fatal(err)
	}
	log.Println("unpacked", msg2)
	if msg.Cmd != msg2.Cmd || msg.Network != msg2.Network || msg.Address != msg2.Address || msg.Cid != msg2.Cid || msg.Wid != msg2.Wid || string(msg.Data) != string(msg2.Data) {
		t.Error("msg and msg2 does not match", msg, msg2)
	}
}

func TestPackerUnpackRejectsMalformedPayload(t *testing.T) {
	p := Packer{Password: "pass123"}
	if msg, err := p.Unpack([]byte{1, 2, 3}); err == nil {
		t.Fatalf("expected malformed payload error, got msg=%+v", msg)
	}
}

func TestPackerRoundTripSmallPayloads(t *testing.T) {
	p := Packer{Password: "pass123"}
	for i := 0; i < 1000; i++ {
		msg := Message{Cmd: DATA, Cid: "cid", Wid: "wid", Data: []byte{byte(i)}}
		data, err := p.Pack(&msg)
		if err != nil {
			t.Fatal(err)
		}
		msg2, err := p.Unpack(data)
		if err != nil {
			t.Fatal(err)
		}
		if msg.Cmd != msg2.Cmd || msg.Cid != msg2.Cid || msg.Wid != msg2.Wid || !bytes.Equal(msg.Data, msg2.Data) {
			t.Fatalf("msg and msg2 do not match: %+v %+v", msg, msg2)
		}
	}
}

func TestPackerUnpackLegacyGobPayload(t *testing.T) {
	msg := Message{Cmd: DATA, Network: "tcp", Address: "example.com:80", Cid: "cid", Wid: "wid", Data: []byte("legacy")}
	p := Packer{Password: "pass123"}
	buf := bytes.Buffer{}
	if err := gob.NewEncoder(&buf).Encode(&msg); err != nil {
		t.Fatal(err)
	}
	data, err := p.Obfuscate(p.Encrypt(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	msg2, err := p.Unpack(data)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Cmd != msg2.Cmd || msg.Network != msg2.Network || msg.Address != msg2.Address || msg.Cid != msg2.Cid || msg.Wid != msg2.Wid || !bytes.Equal(msg.Data, msg2.Data) {
		t.Fatalf("msg and msg2 do not match: %+v %+v", msg, msg2)
	}
}
