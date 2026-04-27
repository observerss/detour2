package common

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"math/rand"

	"github.com/observerss/detour2/crypto/shuffle"
	"github.com/observerss/detour2/crypto/xxtea"
)

const (
	MAGIC_NUMBER      = 31
	KEY_LENGTH        = 16
	MIN_INPUT_LENGTH  = 96
	MAX_TARGET_LENGTH = 192
	MESSAGE_VERSION   = 1
	MAX_STRING_LENGTH = 1<<16 - 1
	MAX_DATA_LENGTH   = 1<<32 - 1
)

type Packer struct {
	Password string
}

func (p *Packer) Pack(msg *Message) ([]byte, error) {
	buf, err := EncodeMessage(msg)
	if err != nil {
		return nil, err
	}
	data, err := p.Obfuscate(p.Encrypt(buf))
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (p *Packer) Unpack(data []byte) (*Message, error) {
	data, err := p.Deobfuscate(data)
	if err != nil {
		return nil, err
	}
	buf := p.Decrypt(data)
	return DecodeMessage(buf)
}

func EncodeMessage(msg *Message) ([]byte, error) {
	buf := bytes.Buffer{}
	buf.WriteByte(MESSAGE_VERSION)
	buf.WriteByte(byte(msg.Cmd))
	if msg.Ok {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
	if err := writeString(&buf, msg.Wid); err != nil {
		return nil, err
	}
	if err := writeString(&buf, msg.Cid); err != nil {
		return nil, err
	}
	if err := writeString(&buf, msg.Msg); err != nil {
		return nil, err
	}
	if err := writeString(&buf, msg.Network); err != nil {
		return nil, err
	}
	if err := writeString(&buf, msg.Address); err != nil {
		return nil, err
	}
	if err := writeBytes(&buf, msg.Data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func DecodeMessage(input []byte) (*Message, error) {
	if len(input) > 0 && input[0] == MESSAGE_VERSION {
		msg, err := decodeBinaryMessage(input)
		if err == nil {
			return msg, nil
		}
		legacyMsg, legacyErr := decodeGobMessage(input)
		if legacyErr == nil {
			return legacyMsg, nil
		}
		return nil, err
	}
	return decodeGobMessage(input)
}

func decodeBinaryMessage(input []byte) (*Message, error) {
	reader := bytes.NewReader(input)
	version, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	if version != MESSAGE_VERSION {
		return nil, fmt.Errorf("unsupported message version %d", version)
	}
	cmd, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	ok, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	wid, err := readString(reader)
	if err != nil {
		return nil, err
	}
	cid, err := readString(reader)
	if err != nil {
		return nil, err
	}
	message, err := readString(reader)
	if err != nil {
		return nil, err
	}
	network, err := readString(reader)
	if err != nil {
		return nil, err
	}
	address, err := readString(reader)
	if err != nil {
		return nil, err
	}
	data, err := readBytes(reader)
	if err != nil {
		return nil, err
	}
	if reader.Len() != 0 {
		return nil, errors.New("message has trailing data")
	}
	return &Message{
		Cmd:     CMD(cmd),
		Wid:     wid,
		Cid:     cid,
		Ok:      ok != 0,
		Msg:     message,
		Network: network,
		Address: address,
		Data:    data,
	}, nil
}

func decodeGobMessage(input []byte) (*Message, error) {
	dec := gob.NewDecoder(bytes.NewBuffer(input))
	msg := Message{}
	if err := dec.Decode(&msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func writeString(buf *bytes.Buffer, value string) error {
	if len(value) > MAX_STRING_LENGTH {
		return fmt.Errorf("message string is too long: %d", len(value))
	}
	var size [2]byte
	binary.BigEndian.PutUint16(size[:], uint16(len(value)))
	buf.Write(size[:])
	buf.WriteString(value)
	return nil
}

func readString(reader *bytes.Reader) (string, error) {
	var size [2]byte
	if _, err := io.ReadFull(reader, size[:]); err != nil {
		return "", err
	}
	data := make([]byte, binary.BigEndian.Uint16(size[:]))
	if _, err := io.ReadFull(reader, data); err != nil {
		return "", err
	}
	return string(data), nil
}

func writeBytes(buf *bytes.Buffer, value []byte) error {
	if uint64(len(value)) > MAX_DATA_LENGTH {
		return fmt.Errorf("message data is too long: %d", len(value))
	}
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(value)))
	buf.Write(size[:])
	buf.Write(value)
	return nil
}

func readBytes(reader *bytes.Reader) ([]byte, error) {
	var size [4]byte
	if _, err := io.ReadFull(reader, size[:]); err != nil {
		return nil, err
	}
	data := make([]byte, binary.BigEndian.Uint32(size[:]))
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, err
	}
	return data, nil
}

func (p *Packer) Encrypt(input []byte) []byte {
	return xxtea.Encrypt(input, []byte(p.Password))
}

func (p *Packer) Decrypt(input []byte) []byte {
	return xxtea.Decrypt(input, []byte(p.Password))
}

func (p *Packer) Obfuscate(input []byte) ([]byte, error) {
	buf := bytes.NewBuffer([]byte{})
	buf.Grow(len(input) + KEY_LENGTH)
	key, err := GenerateRandomBytes(KEY_LENGTH)
	if err != nil {
		return nil, err
	}
	buf.Write(key)
	offset := Max(binary.LittleEndian.Uint16(key[3:5]), uint16(MAX_TARGET_LENGTH+MAGIC_NUMBER)) - MAX_TARGET_LENGTH
	if len(input) >= MIN_INPUT_LENGTH {
		if err := binary.Write(buf, binary.BigEndian, uint16(offset)); err != nil {
			return nil, err
		}
	} else {
		add := MIN_INPUT_LENGTH - len(input)
		if MAX_TARGET_LENGTH > MIN_INPUT_LENGTH {
			add += rand.Intn(MAX_TARGET_LENGTH - MIN_INPUT_LENGTH + 1)
		}
		if err := binary.Write(buf, binary.BigEndian, offset+uint16(add)); err != nil {
			return nil, err
		}
		padding, err := GenerateRandomBytes(add)
		if err != nil {
			return nil, err
		}
		buf.Write(padding)
	}
	buf.Write(shuffle.Encrypt(input, key))
	out := buf.Bytes()
	return out, nil
}

func (p *Packer) Deobfuscate(input []byte) ([]byte, error) {
	if len(input) < KEY_LENGTH+2 {
		return nil, errors.New("packed data is too short")
	}
	key := input[:KEY_LENGTH]
	offset := Max(binary.LittleEndian.Uint16(key[3:5]), uint16(MAX_TARGET_LENGTH+MAGIC_NUMBER)) - MAX_TARGET_LENGTH
	packedPadlen := binary.BigEndian.Uint16(input[KEY_LENGTH : KEY_LENGTH+2])
	if packedPadlen < offset {
		return nil, errors.New("packed data has invalid padding length")
	}
	padlen := int(packedPadlen - offset)
	bodyStart := KEY_LENGTH + 2 + padlen
	if len(input) < bodyStart {
		return nil, errors.New("packed data is truncated")
	}
	return shuffle.Decrypt(input[bodyStart:], key), nil
}

func Max(x, y uint16) uint16 {
	if x < y {
		return y
	}
	return x
}
