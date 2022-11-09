#syntax=docker/dockerfile:latest
FROM golang:1.19 as builder

WORKDIR /src

ENV GO111MODULE=on
ENV GOPROXY=https://goproxy.cn,direct

COPY . .

RUN --mount=type=cache,id=go_mod,target=/go/pkg/mod \
    --mount=type=cache,id=go_cache,target=/root/.cache/go-build \
    go build -o ./dist/detour .


FROM debian:bullseye as runner

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    sed -i 's/deb.debian.org/mirrors.ustc.edu.cn/g' /etc/apt/sources.list \
    && apt update -y \
    && apt install -y --no-install-recommends ca-certificates

ENV TZ="Asia/Shanghai"

WORKDIR /app
RUN mkdir -p logs

COPY --from=builder /src/dist/detour /app/

CMD ["/app/detour"]