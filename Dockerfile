# 1. 第一阶段，编译二进制可执行文件
FROM golang:1.21-alpine3.18 as build-env

ARG TARGETOS
ARG TARGETARCH

COPY . /app
WORKDIR /app
# 安装依赖
RUN apk add --no-cache git make

# 编译二进制可执行文件
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH make build

# 2. 第二阶段，构建最终镜像
FROM alpine:3.18

ARG TARGETOS
ARG TARGETARCH
ARG PLUGIN_FILENAME=plugin.storage.qiniu.com
ARG CONNECTOR_FILENAME=connector.${PLUGIN_FILENAME}

# 这两个可执行文件由上一阶段编译得到
COPY --from=build-env /app/plugin/${PLUGIN_FILENAME} /usr/local/bin/${PLUGIN_FILENAME}
COPY --from=build-env /app/connector/${CONNECTOR_FILENAME} /usr/local/bin/${CONNECTOR_FILENAME}

# 这些文件直接由仓库提供
COPY docker/${TARGETARCH}/kodofs /usr/local/bin/kodofs
COPY docker/${TARGETARCH}/rclone /usr/local/bin/rclone
COPY docker/kodo-csi-connector.service /csiplugin-connector.service
COPY docker/entrypoint.sh /entrypoint.sh

# 赋予这些文件可执行权限
RUN chmod +x /usr/local/bin/kodofs \
    /usr/local/bin/rclone \
    /usr/local/bin/${PLUGIN_FILENAME} \
    /usr/local/bin/${CONNECTOR_FILENAME} \
    /entrypoint.sh

RUN apk add util-linux --no-cache ca-certificates bash

ENTRYPOINT ["/entrypoint.sh"]
