import asyncio
import pickle
import traceback
import uuid
from typing import Dict

from websockets.exceptions import ConnectionClosed
from websockets.client import connect, WebSocketClientProtocol

from .socks5 import init, send_addr, Socks5Request
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
    cdata = pickle.dumps(Message(cmd="connect", cid=cid, host=req.addr, port=req.port))
    try:
        await remote.send(cdata)
    except:
        try:
            remote = await connect("ws://127.0.0.1:3811", ping_interval=None)
        except:
            print("hanlde, cannot connect to ws")
            writer.close()
            return
        else:
            await remote.send(cdata)

    msg: Message = await queue.get()
    await send_addr(msg.ok, writer)
    if not msg.ok:
        print("handle, connect failed")
        writer.close()
        queues.pop(cid, None)
        return

    asyncio.create_task(copy_remote_to_local(queue, writer))
    asyncio.create_task(copy_local_to_remote(reader, writer, remote, cid, req))


async def copy_local_to_remote(
    reader: asyncio.StreamReader,
    writer: asyncio.StreamWriter,
    remote: WebSocketClientProtocol,
    cid: str,
    req: Socks5Request,
):
    print(cid, "copy-to-remote, start forward local to websocket")
    # local to remote
    while True:
        try:
            handle_can_switch = True
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
            await remote.send(pickle.dumps(msg))
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
            # print("ws, disconneted")
            await asyncio.sleep(0.5)
            # try:
            #     remote = await connect("ws://127.0.0.1:3811", ping_interval=None)
            # except ConnectionRefusedError:
            #     pass
            continue
        except:
            await asyncio.sleep(0.5)
            traceback.print_exc()
            continue

        msg: Message = pickle.loads(data)
        print("ws,", msg.cid, "read", len(msg.data))
        queue = queues.get(msg.cid)
        if queue:
            print(msg.cid, "ws, put ===> queue", msg.cmd, len(msg.data))
            await queue.put(msg)


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
