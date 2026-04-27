# detour2 弯路 2

## 本地多级代理

`detour2` 可以按 `local -> relay -> relay -> target` 的方式串联。最后一级 relay 不配置下一跳，作为出口节点；中间 relay 通过 `-r` 指向下一跳 relay。

```text
client app
	|
	| socks5/http
	v
local :3810
	|
	| websocket
	v
middle relay :3811
	|
	| websocket
	v
exit relay :3812
	|
	| tcp
	v
target service
```

启动顺序建议从出口往入口启动：先启动最后一级出口 relay，再启动中间 relay，最后启动 local。这样每一级启动时都能立刻连到下一跳。

```bash
# 出口 relay，直接访问目标网络
./detour relay -l tcp://0.0.0.0:3812 -p PASSWORD -metrics 127.0.0.1:3912

# 中间 relay，把流量转发到出口 relay
./detour relay -l tcp://0.0.0.0:3811 -r ws://127.0.0.1:3812/ws -p PASSWORD -pool 64 -metrics 127.0.0.1:3911

# 本地代理，连接中间 relay
./detour local -l tcp://127.0.0.1:3810 -r ws://127.0.0.1:3811/ws -p PASSWORD -t socks5 -pool 64 -metrics 127.0.0.1:3910
```

systemd 部署脚本支持在同一台下游机器上同时部署 HTTP 和 SOCKS5 入口。HTTP 角色默认监听 `0.0.0.0:7777`，服务名为 `detour2-http`；SOCKS5 角色默认监听 `0.0.0.0:7776`，服务名为 `detour2-socks5`。旧的 `local` 角色仍保留兼容。

```bash
# HTTP 入口，默认监听 http://0.0.0.0:7777，服务名 detour2-http
bash deploy.sh quote http jy230101 tcp://47.116.180.26:7777

# SOCKS5 入口，默认监听 socks5://0.0.0.0:7776，服务名 detour2-socks5
bash deploy.sh quote socks5 jy230101 tcp://47.116.180.26:7777
```

兼容旧用法：`server` 子命令仍可作为出口节点使用；如果给 `server` 增加 `-r`，行为与 `relay` 相同，作为中间 relay 转发到下一跳。
`-pool` 控制到下一跳的 WebSocket 连接数，默认 64；并发连接多时可以降低单条 WebSocket 上的队头阻塞。
出口节点可以用 `-dns 8.8.8.8:53,1.1.1.1:53` 指定目标域名解析器，避免系统 DNS 把 YouTube/Google 资源解析到出口不可达的 IP。
`-metrics 127.0.0.1:3910` 会开启只读 JSON 指标接口，路径为 `/debug/metrics`。建议绑定到 `127.0.0.1`，再通过 SSH 访问，避免把调试信息暴露到公网。

```bash
curl http://127.0.0.1:3910/debug/metrics
```

本机快速验证可以使用示例脚本：

```bash
make build
bash scripts/run-local-relay-chain.sh
curl --socks5-hostname 127.0.0.1:3810 https://example.com/
```

常见排查：

- local 连接失败：检查 `local -r` 是否指向第一跳 relay 的 `/ws` 地址。
- 中间 relay 连接失败：检查 `relay -r` 是否指向下一跳 relay 的 `/ws` 地址，并确认两端密码一致。
- 能连上但目标不可达：在出口 relay 所在机器上直接访问目标服务，确认出口网络本身可达。
- 多跳链路抖动：先用两级链路验证，再逐级增加 relay；每一级 relay 都可以用 `-d` 打开 debug 日志。
- 线上链路慢：用 `bash scripts/diagnose-chain.sh` 检查入口代理、每一跳 HTTP 探活、出口 DNS 和三台 systemd 的近期异常日志。
- 高并发慢：用 `CONCURRENCY=50 TOTAL=80 bash scripts/bench-proxy.sh` 模拟浏览器同时拉取 YouTube/Google 资源，观察错误率和 p95/p99 尾延迟。
- 稳定性验证：用 `DURATION=600 CONCURRENCY=50 TOTAL=80 bash scripts/stability-proxy.sh` 连续压测 10 分钟，并汇总每轮延迟、错误和三台服务日志。
- 运行期指标：给服务增加 `-metrics 127.0.0.1:3910` 后访问 `/debug/metrics`，查看 active 连接数、WebSocket/relay 池状态、writer 队列长度、消息和错误计数。
