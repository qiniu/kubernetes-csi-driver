VERSION = $(shell git describe --tags HEAD || echo "NO_VERSION_TAG")
COMMIT_ID = $(shell git rev-parse --short HEAD || echo "HEAD")
BUILD_TIME = $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
CONNECTOR_FILENAME = connector.plugin.storage.qiniu.com
PLUGIN_FILENAME = plugin.storage.qiniu.com

DOCKERHUB_ORGANIZATION = kodoproduct
DOCKERHUB_IMAGE = csi-plugin.storage.qiniu.com
DOCKERHUB_TAG = ${VERSION}

RCLONE_VERSION = v1.62.2
KODOFS_VERSION = v3.2.9