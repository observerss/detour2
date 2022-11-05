package common

import (
	"bytes"
	"encoding/gob"
)

type Packer struct {
	Password string
}

func (p *Packer) Pack(msg *Message) ([]byte, error) {
	buf := bytes.Buffer{}
	enc := gob.NewEncoder(&buf)
	enc.Encode(msg)
	return p.Obfuscate(p.Encrypt(buf.Bytes())), nil
}

func (p *Packer) Unpack(data []byte) (*Message, error) {
	buf := p.Decrypt(p.Deobfuscate(data))
	dec := gob.NewDecoder(bytes.NewBuffer(buf))
	msg := Message{}
	dec.Decode(&msg)
	return &msg, nil
}

func (p *Packer) Encrypt(input []byte) []byte {
	return input
}

func (p *Packer) Decrypt(input []byte) []byte {
	return input
}

func (p *Packer) Obfuscate(input []byte) []byte {
	return input
}

func (p *Packer) Deobfuscate(input []byte) []byte {
	return input
}
