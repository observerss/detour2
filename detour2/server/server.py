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
    async with serve(handle, "0.0.0.0", 3811, ping_interval=None):
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
        await send_websocket_safe(conn, msg)
        return

    print(cid, "connect, send ok")
    writers[cid] = writer
    msg.ok = True
    if not await send_websocket_safe(conn, msg):
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
    host = msg.host

    print(cid, "loop, start")
    while True:
        cmd = "data"
        try:
            print(cid, "loop wait read")
            data = await asyncio.wait_for(reader.read(16384), timeout=60)
            # data = await reader.read(16384)
            print(cid, "loop, data <=== website", host, len(data))
        except asyncio.exceptions.TimeoutError:
            print(cid, "loop read timed out")
            cmd = "close"
        except:
            traceback.print_exc()
            cmd = "close"
        else:
            if len(data) == 0:
                cmd = "close"
            msg = Message(cmd=cmd, cid=cid, data=data, host=msg.host, port=msg.port)
            print(cid, "loop, send ===> websocket", cmd, len(data))
            await send_websocket_safe(conn, msg)

        if cmd == "close":
            writer.close()
            print(cid, "loop, current num of writers", len(writers), writers.keys())

            # if num == 1 we are closing the last connection
            if len(writers) == 1:
                print("!!!! close websocket now")
                try:
                    await conn.close()
                except:
                    traceback.print_exc()

            # quit for loop
            break

    writers.pop(cid, None)
    print(cid, "loop, quit")


async def handle_data(conn: WebSocketServerProtocol, msg: Message):
    cid = msg.cid
    cmsg = Message(cmd="close", cid=cid, host=msg.host, port=msg.port)
    writer = writers.get(cid)
    if not writer:
        print("data,", msg.cid, "not found")

        print(cid, "reconnect, open connection", msg.host, msg.port)
        try:
            reader, writer = await asyncio.open_connection(msg.host, msg.port)
        except Exception as e:
            traceback.print_exc()
            await send_websocket_safe(conn, cmsg)
            return
        else:
            asyncio.create_task(loop(conn, msg, reader, writer))

    print(cid, "data, send ===> website", len(msg.data))
    writer.write(msg.data)
    try:
        await writer.drain()
    except:
        traceback.print_exc()
        writers.pop(cid)
        await send_websocket_safe(conn, cmsg)


async def handle_close(conn: WebSocketServerProtocol, msg: Message):
    writer = writers.get(msg.cid)
    if writer:
        writer.close()


async def send_websocket_safe(conn: WebSocketServerProtocol, msg: Message) -> bool:
    try:
        conn
        await conn.send(pickle.dumps(msg))
    except ConnectionClosed:
        return False
    except:
        traceback.print_exc()
        return False
    return True


def main():
    asyncio.run(run_server())
