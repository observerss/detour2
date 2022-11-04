from dataclasses import dataclass


@dataclass
class Message:
    cmd: str  # connect/data/close
    cid: str = ""
    ok: bool = True
    msg: str = ""
    host: str = ""
    port: int = 0
    data: bytes = b""
