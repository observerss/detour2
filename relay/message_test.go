package relay

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
)

func Test_MESSAGE(t *testing.T) {
	id, _ := uuid.NewUUID()
	msg := &RelayMessage{
		Pair: &ConnPair{ClientId: id, ConnId: id},
		Data: &RelayData{CMD: CONNECT, Network: "abcd", Address: "efgh"},
	}

	password := "password"

	data := Pack(msg, password)
	fmt.Println(data)

	msg2, _ := Unpack(data, password)

	if msg2.Data.CMD != CONNECT || msg2.Data.Network != "abcd" || msg2.Data.Address != "efgh" {
		t.Error("not match")
		t.Error(msg2.Data)
	}
}
