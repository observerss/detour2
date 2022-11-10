package common

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"math/rand"

	"github.com/observerss/detour2/crypto/shuffle"
	"github.com/observerss/detour2/crypto/xxtea"
)

const (
	MAGIC_NUMBER      = 31
	KEY_LENGTH        = 16
	MIN_INPUT_LENGTH  = 384
	MAX_TARGET_LENGTH = 792
)

type Packer struct {
	Password string
}

func (p *Packer) Pack(msg *Message) ([]byte, error) {
	buf := bytes.Buffer{}
	enc := gob.NewEncoder(&buf)
	enc.Encode(msg)
	data := p.Obfuscate(p.Encrypt(buf.Bytes()))
	return data, nil
}

func (p *Packer) Unpack(data []byte) (*Message, error) {
	buf := p.Decrypt(p.Deobfuscate(data))
	dec := gob.NewDecoder(bytes.NewBuffer(buf))
	msg := Message{}
	dec.Decode(&msg)
	return &msg, nil
}

func (p *Packer) Encrypt(input []byte) []byte {
	return xxtea.Encrypt(input, []byte(p.Password))
}

func (p *Packer) Decrypt(input []byte) []byte {
	return xxtea.Decrypt(input, []byte(p.Password))
}

func (p *Packer) Obfuscate(input []byte) []byte {
	buf := bytes.NewBuffer([]byte{})
	buf.Grow(len(input) + KEY_LENGTH)
	key, _ := GenerateRandomBytes(KEY_LENGTH)
	buf.Write(key)
	offset := Max(binary.LittleEndian.Uint16(key[3:5]), uint16(MAX_TARGET_LENGTH+MAGIC_NUMBER)) - MAX_TARGET_LENGTH
	if len(input) >= MIN_INPUT_LENGTH {
		binary.Write(buf, binary.BigEndian, uint16(offset))
	} else {
		add := rand.Intn(MAX_TARGET_LENGTH-len(input)) + MIN_INPUT_LENGTH - len(input)
		binary.Write(buf, binary.BigEndian, offset+uint16(add))
		padding, _ := GenerateRandomBytes(add)
		buf.Write(padding)
	}
	buf.Write(shuffle.Encrypt(input, key))
	out := buf.Bytes()
	return out
}

func (p *Packer) Deobfuscate(input []byte) []byte {
	buf := bytes.NewBuffer(input)
	key := buf.Next(KEY_LENGTH)
	offset := Max(binary.LittleEndian.Uint16(key[3:5]), uint16(MAX_TARGET_LENGTH+MAGIC_NUMBER)) - MAX_TARGET_LENGTH
	padlen := binary.BigEndian.Uint16(buf.Next(2)) - offset
	buf.Next(int(padlen))
	return shuffle.Decrypt(buf.Bytes(), key)
}

func Max(x, y uint16) uint16 {
	if x < y {
		return y
	}
	return x
}
