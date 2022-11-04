#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
Socks5 Server
"""
import enum
import struct
import socket
import asyncio
from dataclasses import dataclass
from typing import Any, Callable, Coroutine


RESERVED = 0
EMPTY_IP = 0
EMPTY_PORT = 0


class Version(int, enum.Enum):
    SOCKS5 = 5


class AuthType(int, enum.Enum):
    NO_AUTH = 0
    GSSAPI = 1
    USERNAME_PASSWORD = 2
    CHALLENGE_HANDSHAKE = 3
    UNASSIGNED = 4
    CHALLENGE_RESPONSE = 5
    SECURE_SOCKETS_LAYER = 6
    NDS_AUTHENTICATION = 7
    MULTI_AUTHENTICATION_FRAMEWORK = 8
    JSON_PARAMETER_BLOCK = 9


class CMD(int, enum.Enum):
    CONNECT = 1
    BIND = 2
    UDP = 3


class AddrType(int, enum.Enum):
    IPv4 = 1
    DOMAIN = 3
    IPv6 = 4


NO_ACCEPTABLE_METHOD = struct.pack("!BB", Version.SOCKS5, 255)
ADDRESS_TYPE_NOT_SUPPPORTED = struct.pack(
    "!BBBBIH", Version.SOCKS5, 8, RESERVED, AddrType.IPv4, EMPTY_IP, EMPTY_PORT
)
COMMAND_NOT_SUPPPORTED = struct.pack(
    "!BBBBIH", Version.SOCKS5, 7, RESERVED, AddrType.IPv4, EMPTY_IP, EMPTY_PORT
)
GENERAL_FAILURE = struct.pack(
    "!BBBBIH", Version.SOCKS5, 1, RESERVED, AddrType.IPv4, EMPTY_IP, EMPTY_PORT
)
GENERAL_OK = struct.pack(
    "!BBBBIH", Version.SOCKS5, 0, RESERVED, AddrType.IPv4, EMPTY_IP, EMPTY_PORT
)
CONNECTION_REFUSED = struct.pack(
    "!BBBBIH", Version.SOCKS5, 5, RESERVED, AddrType.IPv4, EMPTY_IP, EMPTY_PORT
)


@dataclass
class Socks5Request:
    cmd: CMD
    addr: str
    port: int


@dataclass
class Address:
    type: AddrType
    addr: str
    port: int

    def to_bytes(self):
        if self.type == AddrType.IPv4:
            addr = socket.inet_aton(self.addr)
            return struct.pack(
                "!BBBB4sH", Version.SOCKS5, 0, 0, AddrType.IPv4, addr, self.port
            )
        else:
            raise NotImplementedError("")


async def init(
    reader: asyncio.StreamReader,
    writer: asyncio.StreamWriter,
) -> Socks5Request:
    peername = writer.get_extra_info("peername")
    sockname = writer.get_extra_info("sockname")
    listen = f"tcp://{sockname[0]}:{sockname[1]}"
    peer = f"tcp://{peername[0]}:{peername[1]}"

    ok = await handshake(reader, writer)
    if not ok:
        return False

    req = await get_request(reader, writer)
    if not req:
        return False
    return req


async def send_addr(ok: bool, writer: asyncio.StreamWriter):
    if not ok:
        writer.write(GENERAL_FAILURE)
    else:
        writer.write(GENERAL_OK)

    await writer.drain()


async def handshake(reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> bool:
    try:
        header = await reader.read(2)
        version, nmethods = struct.unpack("!BB", header)
    except struct.error:
        # client connects and disconnect immediately
        return False

    if version != Version.SOCKS5 or nmethods <= 0:
        writer.write(NO_ACCEPTABLE_METHOD)
        return False

    # get available methods
    methods = set()
    for _ in range(nmethods):
        methods.add(ord(await reader.read(1)))

    if AuthType.NO_AUTH in methods:
        writer.write(struct.pack("!BB", Version.SOCKS5, AuthType.NO_AUTH))
        return True
    else:
        writer.write(NO_ACCEPTABLE_METHOD)
        return False


async def get_request(
    reader: asyncio.StreamReader, writer: asyncio.StreamWriter
) -> Socks5Request | None:
    version, cmd, _, address_type = struct.unpack("!BBBB", await reader.read(4))
    assert version == Version.SOCKS5

    if address_type == AddrType.IPv4:
        address = socket.inet_ntoa(await reader.read(4))
    elif address_type == AddrType.IPv6:
        address = socket.inet_ntop(socket.AF_INET6, await reader.read(16))
    elif address_type == AddrType.DOMAIN:
        domain_length = ord(await reader.read(1))
        address = await reader.read(domain_length)
        address = address.decode()
    else:
        writer.write(ADDRESS_TYPE_NOT_SUPPPORTED)
        return

    port = struct.unpack("!H", await reader.read(2))[0]

    if cmd == CMD.CONNECT:  # CONNECT
        return Socks5Request(cmd=CMD.CONNECT, addr=address, port=port)
    else:
        writer.write(COMMAND_NOT_SUPPPORTED)
        return
