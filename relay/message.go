package relay

import (
	"detour/utils/crypto/shuffle"
	"detour/utils/crypto/xxtea"
	"encoding/binary"
	"math/rand"

	"github.com/google/uuid"
)

const KEY_LENGTH = 128
const MIN_DATA_LENGTH = 500
const RAND_DATA_MAX = 1000

type RelayMessage struct {
	Pair *ConnPair  `json:"pair,omitempty"`
	Data *RelayData `json:"data,omitempty"`
}

func Btoi(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func Itob(n byte) bool {
	return n != 0
}

func PackData(data *RelayData) []byte {
	// buf := bytes.Buffer{}
	// encoder := gob.NewEncoder(&buf)
	// encoder.Encode(data)
	// out := buf.Bytes()
	res := make([]byte, 0)

	res = append(res, byte(data.CMD))

	res = append(res, Btoi(data.OK))

	msg := []byte(data.MSG)
	res = binary.BigEndian.AppendUint16(res, uint16(len(msg)))
	res = append(res, msg...)

	msg = []byte(data.Network)
	res = binary.BigEndian.AppendUint16(res, uint16(len(msg)))
	res = append(res, msg...)

	msg = []byte(data.Address)
	res = binary.BigEndian.AppendUint16(res, uint16(len(msg)))
	res = append(res, msg...)

	res = binary.BigEndian.AppendUint16(res, uint16(len(data.Data)))
	res = append(res, data.Data...)

	// data2 := UnpackData(res)
	// if string(data.Data) != string(data2.Data) {
	// 	go func() {
	// 		time.Sleep(time.Millisecond * 100)
	// 		log.Fatal("data not match", data.Data[:100], data2.Data[:100])
	// 	}()
	// }
	return res
}

func UnpackData(input []byte) *RelayData {
	data := RelayData{}

	data.CMD = Command(input[0])
	data.OK = Itob(input[1])
	input = input[2:]

	length := int(binary.BigEndian.Uint16(input[:2]))
	input = input[2:]
	data.MSG = string(input[length])
	input = input[length:]

	length = int(binary.BigEndian.Uint16(input[:2]))
	input = input[2:]
	data.Network = string(input[:length])
	input = input[length:]

	length = int(binary.BigEndian.Uint16(input[:2]))
	input = input[2:]
	data.Address = string(input[:length])
	input = input[length:]

	length = int(binary.BigEndian.Uint16(input[:2]))
	input = input[2:]
	data.Data = []byte(input[:length])

	return &data
}

func Pack(msg *RelayMessage, password string) []byte {
	res := make([]byte, 0)
	res = append(res, (msg.Pair.ClientId[:])...)
	res = append(res, (msg.Pair.ConnId[:])...)

	// key
	token := make([]byte, KEY_LENGTH)
	rand.Read(token)
	key := xxtea.Encrypt(token, []byte(password))
	res = binary.BigEndian.AppendUint16(res, uint16(len(key)))
	res = append(res, key...)

	// padding
	data := PackData(msg.Data)
	if len(data) < MIN_DATA_LENGTH {
		length := uint16(rand.Intn(RAND_DATA_MAX - len(data)))
		res = binary.BigEndian.AppendUint16(res, length)
		padding := make([]byte, length)
		rand.Read(padding)
		res = append(res, padding...)
	} else {
		res = binary.BigEndian.AppendUint16(res, 0)
	}

	// data
	data = shuffle.Encrypt(data, token)
	// log.Println(len(token))
	res = binary.BigEndian.AppendUint32(res, uint32(len(data)))
	res = append(res, data...)

	return res
}

func Unpack(input []byte, password string) (*RelayMessage, error) {
	var (
		clientId uuid.UUID
		connId   uuid.UUID
		key      []byte
		token    []byte
		data     []byte
		err      error
		length   uint32
	)

	if clientId, err = uuid.FromBytes(input[:16]); err != nil {
		return nil, err
	}
	input = input[16:]
	if connId, err = uuid.FromBytes(input[:16]); err != nil {
		return nil, err
	}
	input = input[16:]

	// key
	length = uint32(binary.BigEndian.Uint16(input[:2]))
	input = input[2:]
	key = input[:length]
	token = xxtea.Decrypt(key, []byte(password))
	input = input[length:]

	// padding
	length = uint32(binary.BigEndian.Uint16(input[:2]))
	input = input[2:]
	input = input[length:] // ignore padding

	// data
	length = uint32(binary.BigEndian.Uint32(input[:4]))
	input = input[4 : 4+length]
	data = shuffle.Decrypt(input, token)
	// data = input
	// log.Println(len(token))

	msg := RelayMessage{
		Pair: &ConnPair{ClientId: clientId, ConnId: connId},
	}

	msg.Data = UnpackData(data)
	return &msg, nil
}
