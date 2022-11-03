.PHONY: build image clean connector/connector.plugin.storage.qiniu.com plugin/plugin.storage.qiniu.com

VERSION = v0.0.2
COMMITID = $(shell git rev-parse --short HEAD || echo "HEAD")
BUILDTIME = $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

build: image
connector/connector.plugin.storage.qiniu.com:
	cd connector && \
		go build -ldflags "-X main.VERSION=$(VERSION) -X main.COMMITID=$(COMMITID) -X main.BUILDTIME=$(BUILDTIME)" -o connector.plugin.storage.qiniu.com
plugin/plugin.storage.qiniu.com:
	cd plugin && \
		go build -ldflags "-X main.VERSION=$(VERSION) -X main.COMMITID=$(COMMITID) -X main.BUILDTIME=$(BUILDTIME)" -o plugin.storage.qiniu.com
image: connector/connector.plugin.storage.qiniu.com plugin/plugin.storage.qiniu.com
	cp plugin/plugin.storage.qiniu.com docker/
	cp connector/connector.plugin.storage.qiniu.com docker/
	docker build --pull -t="bachue/csi-plugin.storage.qiniu.com:${VERSION}-${COMMITID}" docker/
	docker push "bachue/csi-plugin.storage.qiniu.com:${VERSION}-${COMMITID}"
clean:
	rm -f connector/connector.plugin.storage.qiniu.com plugin/plugin.storage.qiniu.com docker/plugin.storage.qiniu.com docker/connector.plugin.storage.qiniu.com
