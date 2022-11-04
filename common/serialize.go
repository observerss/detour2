package common

func Pack(msg *Message) ([]byte, error) {
	return []byte{}, nil
}

func Unpack(data []byte) (*Message, error) {
	return &Message{}, nil
}
