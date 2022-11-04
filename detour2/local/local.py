import asyncio
import pickle
import traceback
import uuid
from typing import Dict

from websockets.exceptions import ConnectionClosed
from websockets.client import connect, WebSocketClientProtocol

from .socks5 import init, send_addr, Address, AddrType
from ..schema import Message


remote: WebSocketClientProtocol = None
queues: Dict[str, asyncio.Queue] = {}
writers: Dict[str, asyncio.StreamWriter] = {}


async def run_local():
    server = await asyncio.start_server(handle_socks5, "0.0.0.0", 3810)
    asyncio.create_task(run_remote())
    print("Listening at :3810")
    await server.serve_forever()


async def handle_socks5(reader: asyncio.StreamReader, writer: asyncio.StreamWriter):
    global remote
    cid = str(uuid.uuid4())[:8]
    print(cid, "init")
    queue = queues[cid] = asyncio.Queue()
    queue.cid = cid
    req = await init(reader, writer)
    if not req:
        writer.close()
        return

    print(cid, "handle, send connect")
    await remote.send(
        pickle.dumps(Message(cmd="connect", cid=cid, host=req.addr, port=req.port))
    )
    msg: Message = await queue.get()
    await send_addr(msg.ok, writer)
    if not msg.ok:
        print("handle, connect failed")
        writer.close()
        queues.pop(cid, None)
        return

    asyncio.create_task(copy_remote_to_local(queue, writer))

    print(cid, "handle, start forward local to websocket")
    # local to remote
    while True:
        try:
            data = await reader.read(16384)
            print(cid, "handle, read <=== local", len(data))
        except ConnectionAbortedError:
            print(cid, "handle, connection abort")
            break
        except:
            traceback.print_exc()
            break

        msg = Message(cmd="data", cid=cid, host=req.addr, port=req.port, data=data)
        if len(data) == 0:
            msg.cmd = "close"

        try:
            print(cid, "handle, send ===> websocket", msg.cmd, len(msg.data))
            await remote.send(pickle.dumps(msg))
        except ConnectionClosed:
            print(cid, "handle, connection closed")
            break
        except:
            traceback.print_exc()
            break

        if len(data) == 0:
            break

    writer.close()
    print(cid, "handle quit")


async def run_remote():
    global remote
    print("ws, run")
    while True:
        try:
            print("ws, wait read")
            data = await remote.recv()
        except (
            GeneratorExit,
            RuntimeError,
            KeyboardInterrupt,
        ):
            break
        except (AttributeError, ConnectionClosed):
            print("ws, reconnect")
            try:
                remote = await connect("ws://127.0.0.1:3811")
            except ConnectionRefusedError:
                pass
            continue
        except:
            traceback.print_exc()
            continue

        msg: Message = pickle.loads(data)
        print("ws,", msg.cid, "read", len(msg.data))
        queue = queues.get(msg.cid)
        if queue:
            print(msg.cid, "ws, put ===> queue", msg.cmd, len(msg.data))
            await queue.put(msg)


async def copy_remote_to_local(queue: asyncio.Queue, writer: asyncio.StreamWriter):
    print(queue.cid, "copy, start")
    while True:
        msg: Message = await queue.get()
        print(queue.cid, "copy, get <=== queue", msg.cmd, len(msg.data))
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
            except:
                traceback.print_exc()
    print(queue.cid, "copy, quit")


def main():
    asyncio.run(run_local())
