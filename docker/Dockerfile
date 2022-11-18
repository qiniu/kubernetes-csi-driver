FROM debian:bullseye

ARG KODOFS_VERSION=2.4.18
ARG RCLONE_VERSION=1.60.1
COPY nsenter /usr/local/bin/nsenter
COPY kodofs-v${KODOFS_VERSION} /usr/local/bin/kodofs
COPY rclone-v${RCLONE_VERSION} /usr/local/bin/rclone
COPY kodo-csi-connector.service /csiplugin-connector.service
COPY plugin.storage.qiniu.com /usr/local/bin/plugin.storage.qiniu.com
COPY connector.plugin.storage.qiniu.com /usr/local/bin/connector.plugin.storage.qiniu.com
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /usr/local/bin/kodofs /usr/local/bin/rclone /usr/local/bin/plugin.storage.qiniu.com /usr/local/bin/connector.plugin.storage.qiniu.com /entrypoint.sh
RUN apt-get update -yqq && apt-get install -yqq ca-certificates && rm -rf /var/lib/apt/lists/*

ENTRYPOINT ["/entrypoint.sh"]
