# 1. 第一阶段，编译二进制可执行文件
FROM golang:1.18.10-bullseye as build-env
COPY . /app
WORKDIR /app
RUN apt update -yqq && \
    apt install -yqq git make
RUN make build

# 2. 第二阶段，构建最终镜像
FROM debian:bullseye

ARG KODOFS_VERSION=2.4.18
ARG RCLONE_VERSION=1.60.1
ARG PLUGIN_FILENAME=plugin.storage.qiniu.com
ARG CONNECTOR_FILENAME=connector.${PLUGIN_FILENAME}

# 这两个可执行文件由上一阶段编译得到
COPY --from=build-env /app/plugin/${PLUGIN_FILENAME} /usr/local/bin/${PLUGIN_FILENAME}
COPY --from=build-env /app/connector/${CONNECTOR_FILENAME} /usr/local/bin/${CONNECTOR_FILENAME}

# 这些文件直接由仓库提供
COPY docker/nsenter /usr/local/bin/nsenter
COPY docker/kodofs-v${KODOFS_VERSION} /usr/local/bin/kodofs
COPY docker/rclone-v${RCLONE_VERSION} /usr/local/bin/rclone
COPY docker/kodo-csi-connector.service /csiplugin-connector.service
COPY docker/entrypoint.sh /entrypoint.sh

# 赋予这些文件可执行权限
RUN chmod +x /usr/local/bin/kodofs \
    /usr/local/bin/rclone \
    /usr/local/bin/${PLUGIN_FILENAME} \
    /usr/local/bin/${CONNECTOR_FILENAME} \
    /entrypoint.sh


RUN apt-get update -yqq && \
    apt-get install -yqq ca-certificates && \
    rm -rf /var/lib/apt/lists/*

ENTRYPOINT ["/entrypoint.sh"]
