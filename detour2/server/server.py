#!/usr/bin/env python
import pickle
import asyncio
import traceback

from typing import Dict
from websockets.server import serve, WebSocketServerProtocol
from websockets.exceptions import ConnectionClosed

from ..schema import Message


writers: Dict[str, asyncio.StreamWriter] = {}


async def run_server():
    print("Listen on :3810")
    async with serve(handle, "0.0.0.0", 3811):
        await asyncio.Future()  # run forever


async def handle(conn: WebSocketServerProtocol):
    while True:
        try:
            print("ws, wait read")
            message = await conn.recv()
            msg: Message = pickle.loads(message)
            print(msg.cid, "ws, read", msg.cmd, len(msg.data))

            if msg.cmd == "connect":
                await handle_connect(conn, msg)
            elif msg.cmd == "data":
                await handle_data(conn, msg)
            elif msg.cmd == "close":
                await handle_close(conn, msg)
            else:
                print("handle,", msg.cmd, "not implemented")
        except ConnectionClosed:
            break
        except Exception as e:
            traceback.print_exc()
            break

    if not conn.closed:
        await conn.close()


async def handle_connect(conn: WebSocketServerProtocol, msg: Message):
    cid = msg.cid
    print(cid, "connect, open connection", msg.host, msg.port)
    try:
        reader, writer = await asyncio.open_connection(msg.host, msg.port)
    except Exception as e:
        traceback.print_exc()
        msg.ok = False
        msg.msg = str(e)
        await conn.send(pickle.dumps(msg))
        return

    print(cid, "connect, send ok")
    writers[cid] = writer
    msg.ok = True
    try:
        await conn.send(pickle.dumps(msg))
    except:
        traceback.print_exc()
        return

    asyncio.create_task(loop(conn, msg, reader, writer))
    print(cid, "connect done")


async def loop(
    conn: WebSocketServerProtocol,
    msg: Message,
    reader: asyncio.StreamReader,
    writer: asyncio.StreamWriter,
):
    cid = msg.cid

    print(cid, "loop, start")
    while True:
        cmd = "data"
        try:
            data = await asyncio.wait_for(reader.read(16384), timeout=60)
            # data = await reader.read(16384)
            print(cid, "loop, data <=== website", len(data))
        except asyncio.exceptions.TimeoutError:
            cmd = "close"
        except:
            traceback.print_exc()
            cmd = "close"
        else:
            if len(data) == 0:
                cmd = "close"
            msg = Message(cmd=cmd, cid=cid, data=data, host=msg.host, port=msg.port)
            print(cid, "loop, send ===> websocket", cmd, len(data))
            try:
                await conn.send(pickle.dumps(msg))
            except:
                traceback.print_exc()

        if cmd == "close":
            writer.close()
            print(cid, "loop, current num of writers", len(writers))

            # if num == 0 we can close websocket altogather
            if len(writers) == 0:
                try:
                    await conn.close()
                except:
                    traceback.print_exc()

            # quit for loop
            break

    writers.pop(cid, None)
    print(cid, "loop, quit")


async def handle_data(conn: WebSocketServerProtocol, msg: Message):
    print("here")
    cid = msg.cid
    cmsg = Message(cmd="close", cid=cid, host=msg.host, port=msg.port)
    writer = writers.get(cid)
    if not writer:
        print("data,", msg.cid, "not found")
        # fmsg = Message(cmd="connect", cid=cid, host=msg.host, port=msg.port)
        # await asyncio.create_task(handle_connect(conn, msg))
        try:
            await conn.send(pickle.dumps(cmsg))
        except:
            traceback.print_exc()
        return

    print(cid, "data, send ===> website", len(msg.data))
    writer.write(msg.data)
    try:
        await writer.drain()
    except:
        traceback.print_exc()
        writers.pop(cid)
        try:
            await conn.send(pickle.dumps(cmsg))
        except:
            traceback.print_exc()


async def handle_close(conn: WebSocketServerProtocol, msg: Message):
    writer = writers.get(msg.cid)
    if writer:
        writer.close()


def main():
    asyncio.run(run_server())
