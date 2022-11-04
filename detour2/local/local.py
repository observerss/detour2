import asyncio
import pickle
import time
import traceback
import uuid
import struct
from typing import Dict

from websockets.exceptions import ConnectionClosed
from websockets.client import connect, WebSocketClientProtocol

from .socks5 import init, send_addr, Socks5Request
from ..schema import Message


connected = False
remote_read: WebSocketClientProtocol = None
remote_send: WebSocketClientProtocol = None
queues: Dict[str, asyncio.Queue] = {}
writers: Dict[str, asyncio.StreamWriter] = {}
switch_read = asyncio.Lock()
switch_send = asyncio.Lock()


async def run_local():
    server = await asyncio.start_server(handle_socks5, "0.0.0.0", 3810)
    asyncio.create_task(run_remote())
    asyncio.create_task(run_switch())
    print("Listening at :3810")
    await server.serve_forever()


async def handle_socks5(reader: asyncio.StreamReader, writer: asyncio.StreamWriter):
    global remote_read, remote_send, connected

    cid = str(uuid.uuid4())[:8]
    print(cid, "init")
    queue = queues[cid] = asyncio.Queue()
    queue.cid = cid
    try:
        req = await init(reader, writer)
    except struct.error:
        writer.close()
        return

    if not req:
        writer.close()
        return

    print(cid, "handle, send connect")
    cdata = pickle.dumps(Message(cmd="connect", cid=cid, host=req.addr, port=req.port))
    try:
        async with switch_send:
            await remote_send.send(cdata)
    except:
        try:
            async with switch_send:
                remote_read = remote_send = await connect(
                    "ws://127.0.0.1:3811", ping_interval=None
                )
        except:
            print("hanlde, cannot connect to ws")
            writer.close()
            return
        else:
            connected = True
            async with switch_send:
                await remote_send.send(cdata)

    msg: Message = await queue.get()
    await send_addr(msg.ok, writer)
    if not msg.ok:
        print("handle, connect failed")
        writer.close()
        queues.pop(cid, None)
        return

    asyncio.create_task(copy_remote_to_local(queue, writer))
    asyncio.create_task(copy_local_to_remote(reader, writer, cid, req))


async def copy_local_to_remote(
    reader: asyncio.StreamReader,
    writer: asyncio.StreamWriter,
    cid: str,
    req: Socks5Request,
):
    global remote_read

    print(cid, "copy-to-remote, start forward local to websocket")
    # local to remote
    while True:
        try:
            data = await reader.read(16384)
            print(cid, "copy-to-remote, read <=== local", len(data))
        except ConnectionAbortedError:
            print(cid, "copy-to-remote, connection abort")
            break
        except:
            traceback.print_exc()
            break

        msg = Message(cmd="data", cid=cid, host=req.addr, port=req.port, data=data)
        if len(data) == 0:
            msg.cmd = "close"

        try:
            print(cid, "copy-to-remote, send ===> websocket", msg.cmd, len(msg.data))

            async with switch_send:
                await remote_send.send(pickle.dumps(msg))
        except ConnectionClosed:
            print(cid, "copy-to-remote, connection closed")
            break
        except:
            traceback.print_exc()
            break

        if len(data) == 0:
            break

    writer.close()
    print(cid, "copy-to-remote quit")


async def run_remote():
    global remote_read, connected
    print("ws, run")
    while True:
        try:
            print("ws, wait read")
            async with switch_read:
                data = await remote_read.recv()
        except (
            GeneratorExit,
            RuntimeError,
            KeyboardInterrupt,
        ):
            break
        except (AttributeError, ConnectionClosed):
            # print("ws, disconneted")
            connected = False
            await asyncio.sleep(0.5)
            # try:
            #     remote = await connect("ws://127.0.0.1:3811", ping_interval=None)
            # except ConnectionRefusedError:
            #     pass
            continue
        except:
            connected = False
            traceback.print_exc()
            await asyncio.sleep(0.5)
            continue

        msg: Message = pickle.loads(data)
        print(msg.cid, "ws, read", msg.cmd, len(msg.data))
        queue = queues.get(msg.cid)
        if queue:
            print(msg.cid, "ws, put ===> queue", msg.cmd, len(msg.data))
            await queue.put(msg)


async def run_switch():
    global remote_read, remote_send
    while True:
        await asyncio.sleep(8)
        if connected:
            print("\n".join("switch, start" for _ in range(10)))
            # create connection
            newremote = await connect("ws://127.0.0.1:3811", ping_interval=None)

            async with switch_read:
                async with switch_send:
                    # tell server to switch to new connection
                    print("switch, send switch cmd")
                    await newremote.send(pickle.dumps(Message(cmd="switch")))
                    wait_flush = 0.05

                    while True:
                        try:
                            print("switch, flush from read", wait_flush)
                            data = await asyncio.wait_for(
                                remote_read.recv(), wait_flush
                            )
                        except asyncio.exceptions.TimeoutError:
                            break
                        else:
                            msg: Message = pickle.loads(data)
                            print(msg.cid, "ws, read", msg.cmd, len(msg.data))
                            queue = queues.get(msg.cid)
                            if queue:
                                print(
                                    msg.cid,
                                    "ws, put ===> queue",
                                    msg.cmd,
                                    len(msg.data),
                                )
                                await queue.put(msg)

                    print("switch, close old")
                    await remote_read.close()

                    print("switch, replace sender")
                    remote_send = remote_read = newremote


async def copy_remote_to_local(queue: asyncio.Queue, writer: asyncio.StreamWriter):
    print(queue.cid, "copy-from-remote, start")
    while True:
        msg: Message = await queue.get()
        print(queue.cid, "copy-from-remote, get <=== queue", msg.cmd, len(msg.data))
        if msg.cmd == "close":
            queues.pop(queue.cid, None)
            writer.close()
            break
        elif msg.cmd == "data":
            writer.write(msg.data)
            if len(msg.data) == 0:
                break
            try:
                await writer.drain()
            except ConnectionResetError:
                break
            except:
                traceback.print_exc()
                break
    print(queue.cid, "copy, quit")


def main():
    asyncio.run(run_local())
