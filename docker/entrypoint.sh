#! /usr/bin/env bash

set -e

HOST_CMD="nsenter --all --target 1 --"

rm -f /host/usr/local/bin/kodofs /host/usr/local/bin/connector.plugin.storage.qiniu.com /host/usr/local/bin/rclone
cp /usr/local/bin/kodofs /host/usr/local/bin/kodofs
cp /usr/local/bin/rclone /host/usr/local/bin/rclone
cp /usr/local/bin/connector.plugin.storage.qiniu.com /host/usr/local/bin/connector.plugin.storage.qiniu.com
cp /csiplugin-connector.service /host/etc/systemd/system/csiplugin-connector.service

$HOST_CMD /usr/local/bin/connector.plugin.storage.qiniu.com -test

$HOST_CMD systemctl daemon-reload
$HOST_CMD systemctl enable csiplugin-connector
$HOST_CMD systemctl restart csiplugin-connector

/usr/local/bin/plugin.storage.qiniu.com $@
