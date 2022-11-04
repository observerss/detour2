package relay

type Message struct {
	Cmd     string
	Cid     string
	Ok      bool
	Msg     string
	Network string
	Address string
	Data    []byte
}

type Package struct {
	Key     []byte
	Padding int
	Message *Message
}

// class Message:
//     cmd: str  # connect/data/close
//     cid: str = ""
//     ok: bool = True
//     msg: str = ""
//     host: str = ""
//     port: int = 0
//     data: bytes = b""
