package relay

import (
	"detour/schema"
	"detour/utils/crypto/shuffle"
	"detour/utils/crypto/xxtea"
	"encoding/binary"
	"encoding/json"
	"math/rand"

	"github.com/google/uuid"
)

const KEY_LENGTH = 32
const MIN_DATA_LENGTH = 500
const RAND_DATA_MAX = 1000

type RelayMessage struct {
	Pair schema.ConnPair   `json:"pair,omitempty"`
	Data *schema.RelayData `json:"data,omitempty"`
}

func Pack(msg *RelayMessage, password string) []byte {
	res := make([]byte, 0)
	res = append(res, (msg.Pair.ClientId[:])...)
	res = append(res, (msg.Pair.ConnId[:])...)

	// key
	token := make([]byte, KEY_LENGTH)
	rand.Read(token)
	key := xxtea.Encrypt(token, []byte(password))
	binary.BigEndian.AppendUint16(res, uint16(len(key)))
	res = append(res, key...)

	// padding
	var data []byte
	data, _ = json.Marshal(msg.Data)
	if len(data) < MIN_DATA_LENGTH {
		length := uint16(rand.Intn(RAND_DATA_MAX - len(data)))
		binary.BigEndian.AppendUint16(res, length)
		padding := make([]byte, length)
		rand.Read(padding)
		res = append(res, padding...)
	} else {
		binary.BigEndian.AppendUint16(res, 0)
	}

	// data
	data = shuffle.Encrypt(data, token)
	binary.BigEndian.AppendUint32(res, uint32(len(data)))
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
	length = uint32(binary.BigEndian.Uint16(input[:4]))
	input = input[4:]
	data = shuffle.Decrypt(input[:length], token)

	msg := &RelayMessage{
		Pair: schema.ConnPair{ClientId: clientId, ConnId: connId},
		Data: &schema.RelayData{},
	}
	json.Unmarshal(data, msg.Data)
	return msg, nil
}
