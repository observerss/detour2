# DEPLOY

## 环境&依赖

- 阿里云
  - 开通 cn-hongkong 区域的函数计算服务
  - AccountID(在网页右上角头像浮窗里)
  - 创建访问控制账号并创建 key 和 secret
    - AccessKeyId
    - AccessKeySecret
- 环境
  - 一台 PC/Mac/Linux 计算机
  - Docker 环境: 可以安装[Docker Desktop](https://www.docker.com/products/docker-desktop/)或者[Docker Engine](https://docs.docker.com/engine/install/)

## 部署方式

### 1) 部署函数计算服务

#### **默认方式部署**

```bash
docker run --rm -it \
    registry.cn-hongkong.aliyuncs.com/hjcrocks/detour2:0.3.0 \
    ./detour deploy -m server -k ACCESS_KEY_ID -s ACCESS_KEY_SECRET -a ACCOUNT_ID -p PASSWORD
```

#### **示例输出**

```log
2022/11/09 17:30:05.371563 server.go:14: INFO  deploy on aliyun...
2022/11/09 17:30:05.568179 server.go:22: INFO  create service...
2022/11/09 17:30:05.703150 server.go:28: INFO  create function...
2022/11/09 17:30:06.091199 server.go:41: INFO  create trigger...
2022/11/09 17:30:06.208164 server.go:42: INFO  deploy ok.
    url = https://ef-fd-asdfasdf.cn-hongkong.fcapp.run
    ws = wss://ef-fd-asdfasdf.cn-hongkong.fcapp.run/ws
```

#### **测试服务**

返回的 http url 可以用于测试连接，直接打开或者在 console 请求

```bash
curl https://ef-fd-asdfasdf.cn-hongkong.fcapp.run/
# 2022-11-09T17:35:12+08:00
```

返回当前时间戳说明部署已成功

#### **删除函数服务**

在部署参数后加上`-remove`即可删除部署

```bash
docker run --rm -it \
    registry.cn-hongkong.aliyuncs.com/hjcrocks/detour2:0.3.0 \
    ./detour deploy -m server -k ACCESS_KEY_ID -s ACCESS_KEY_SECRET -a ACCOUNT_ID -remove
```

**部署可选参数**

```usage
Usage of deploy:
  -a string
        aliyun main account id
  -fn string
        aliyun fc function name (default "dt2")
  -i string
        aliyun container registry uri (default "registry-vpc.cn-hongkong.aliyuncs.com/hjcrocks/detour2:0.3.0")
  -k string
        aliyun access key id
  -m string
        deploy 'server' or 'local'
  -p string
        password for authentication (default "password")
  -pp int
        public port to use (default 3810)
  -r string
        aliyun region (default "cn-hongkong")
  -s string
        aliyun access key secret
  -sn string
        aliyun fc service name (default "api2")
  -tn string
        aliyun fc trigger name (default "ws2")
```

### 2) 运行本地代理

#### **本地运行代理**

在本地 3333 端口运行 SOCKS5 代理

```bash
docker run --rm -it \
    -v /var/run/docker.sock:/var/run/docker.sock \
    registry.cn-hongkong.aliyuncs.com/hjcrocks/detour2:0.3.0 \
    ./detour deploy -m local -k ACCESS_KEY_ID -s ACCESS_KEY_SECRET -a ACCOUNT_ID -pp 3333 -p PASSWORD
```

#### **测试代理工作正常**

```bash
# gist地址被墙, curl返回为空
curl https://gist.github.com

# 用了代理就能访问了
curl --head --socks5 localhost:3333 https://gist.github.com
```
