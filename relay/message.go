package relay

import (
	"bytes"
	"encoding/gob"
	"log"
)

const KEY_LENGTH = 128
const MIN_DATA_LENGTH = 500
const RAND_DATA_MAX = 1000

type RelayMessage struct {
	Pair *ConnPair  `json:"pair,omitempty"`
	Data *RelayData `json:"data,omitempty"`
}

func Pack(msg *RelayMessage, password string) []byte {
	buf := &bytes.Buffer{}
	encoder := gob.NewEncoder(buf)
	encoder.Encode(msg)
	data := buf.Bytes()
	return data
}

func Unpack(input []byte, password string) (*RelayMessage, error) {
	msg := &RelayMessage{}
	buf := bytes.NewBuffer(input)
	decoder := gob.NewDecoder(buf)
	decoder.Decode(&msg)
	log.Println(msg.Pair, msg.Data)
	return msg, nil
}

// func Pack(msg *RelayMessage, password string) []byte {
// 	res := make([]byte, 0)
// 	res = append(res, (msg.Pair.ClientId[:])...)
// 	res = append(res, (msg.Pair.ConnId[:])...)

// 	// key
// 	token := make([]byte, KEY_LENGTH)
// 	rand.Read(token)
// 	key := xxtea.Encrypt(token, []byte(password))
// 	res = binary.BigEndian.AppendUint16(res, uint16(len(key)))
// 	res = append(res, key...)

// 	// padding
// 	buf := &bytes.Buffer{}
// 	encoder := gob.NewEncoder(buf)
// 	encoder.Encode(msg.Data)
// 	data := buf.Bytes()
// 	// data, _ := json.Marshal(msg.Data)
// 	if len(data) < MIN_DATA_LENGTH {
// 		length := uint16(rand.Intn(RAND_DATA_MAX - len(data)))
// 		res = binary.BigEndian.AppendUint16(res, length)
// 		padding := make([]byte, length)
// 		rand.Read(padding)
// 		res = append(res, padding...)
// 	} else {
// 		res = binary.BigEndian.AppendUint16(res, 0)
// 	}

// 	// data
// 	data = shuffle.Encrypt(data, token)
// 	// log.Println(len(token))
// 	res = binary.BigEndian.AppendUint32(res, uint32(len(data)))
// 	res = append(res, data...)

// 	return res
// }

// func Unpack(input []byte, password string) (*RelayMessage, error) {
// 	var (
// 		clientId uuid.UUID
// 		connId   uuid.UUID
// 		key      []byte
// 		token    []byte
// 		data     []byte
// 		err      error
// 		length   uint32
// 	)

// 	if clientId, err = uuid.FromBytes(input[:16]); err != nil {
// 		return nil, err
// 	}
// 	input = input[16:]
// 	if connId, err = uuid.FromBytes(input[:16]); err != nil {
// 		return nil, err
// 	}
// 	input = input[16:]

// 	// key
// 	length = uint32(binary.BigEndian.Uint16(input[:2]))
// 	input = input[2:]
// 	key = input[:length]
// 	token = xxtea.Decrypt(key, []byte(password))
// 	input = input[length:]

// 	// padding
// 	length = uint32(binary.BigEndian.Uint16(input[:2]))
// 	input = input[2:]
// 	input = input[length:] // ignore padding

// 	// data
// 	length = uint32(binary.BigEndian.Uint32(input[:4]))
// 	input = input[4 : 4+length]
// 	data = shuffle.Decrypt(input, token)
// 	// data = input
// 	// log.Println(len(token))

// 	msg := &RelayMessage{
// 		Pair: &ConnPair{ClientId: clientId, ConnId: connId},
// 		Data: &RelayData{},
// 	}

// 	buf := bytes.NewBuffer(data)
// 	decoder := gob.NewDecoder(buf)
// 	decoder.Decode(&msg.Data)
// 	// json.Unmarshal(data, msg.Data)
// 	return msg, nil
// }
