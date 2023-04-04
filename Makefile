include common.mk

.PHONY: all
all: build

.PHONY: build
build: connector/$(CONNECTOR_FILENAME) plugin/$(PLUGIN_FILENAME)

connector/$(CONNECTOR_FILENAME):
	cd connector && \
		go build -ldflags \
		"-X main.VERSION=$(VERSION) -X main.COMMITID=$(COMMIT_ID) -X main.BUILDTIME=$(BUILD_TIME)" \
		-o $(CONNECTOR_FILENAME)

plugin/$(PLUGIN_FILENAME):
	cd plugin && \
		go build -ldflags \
		"-X main.VERSION=$(VERSION) -X main.COMMITID=$(COMMIT_ID) -X main.BUILDTIME=$(BUILD_TIME)" \
		-o $(PLUGIN_FILENAME)

.PHONY: clean
clean:
	rm -f connector/$(CONNECTOR_FILENAME) \
		plugin/$(PLUGIN_FILENAME)
	rm -f k8s/kodo.yaml k8s/kodofs.yaml

.PHONY: build_image
build_image:
	docker build --pull \
		-t="$(DOCKERHUB_ORGANIZATION)/csi-$(PLUGIN_FILENAME):$(VERSION)" \
		-f Dockerfile .

.PHONY: push_image
push_image: build_image
	docker push "$(DOCKERHUB_ORGANIZATION)/csi-$(PLUGIN_FILENAME):${VERSION}"

k8s/kodo.yaml: k8s/kodo/kodo-plugin.yaml k8s/kodo/kodo-rbac.yaml k8s/kodo/kodo-provisioner.yaml
	@cat k8s/kodo/kodo-plugin.yaml \
		| sed 's/$${DOCKERHUB_ORGANIZATION}/$(DOCKERHUB_ORGANIZATION)/g' \
		| sed 's/$${DOCKERHUB_IMAGE}/$(DOCKERHUB_IMAGE)/g' \
		| sed 's/$${DOCKERHUB_TAG}/$(DOCKERHUB_TAG)/g' \
		>> k8s/kodo.yaml
	@echo --- >> k8s/kodo.yaml
	@cat k8s/kodo/kodo-rbac.yaml >> k8s/kodo.yaml
	@echo --- >> k8s/kodo.yaml
	@cat k8s/kodo/kodo-provisioner.yaml >> k8s/kodo.yaml

k8s/kodofs.yaml: k8s/kodofs/kodofs-plugin.yaml k8s/kodofs/kodofs-rbac.yaml k8s/kodofs/kodofs-provisioner.yaml
	@cat k8s/kodofs/kodofs-plugin.yaml \
		| sed 's/$${DOCKERHUB_ORGANIZATION}/$(DOCKERHUB_ORGANIZATION)/g' \
		| sed 's/$${DOCKERHUB_IMAGE}/$(DOCKERHUB_IMAGE)/g' \
		| sed 's/$${DOCKERHUB_TAG}/$(DOCKERHUB_TAG)/g' \
		>> k8s/kodofs.yaml
	@echo --- >> k8s/kodofs.yaml
	@cat k8s/kodofs/kodofs-rbac.yaml >> k8s/kodofs.yaml
	@echo --- >> k8s/kodofs.yaml
	@cat k8s/kodofs/kodofs-provisioner.yaml >> k8s/kodofs.yaml

.PHONY: combine_csi_driver_yaml
combine_csi_driver_yaml: k8s/kodo.yaml k8s/kodofs.yaml

.PHONY: install_kodo_csi_driver
install_kodo_csi_driver: k8s/kodo.yaml
	kubectl apply -f $<

.PHONY: install_kodofs_csi_driver
install_kodofs_csi_driver: k8s/kodofs.yaml
	kubectl apply -f $<

.PHONY: delete_kodo_csi_driver
delete_kodo_csi_driver: k8s/kodo.yaml
	kubectl delete -f $<

.PHONY: delete_kodofs_csi_driver
delete_kodofs_csi_driver: k8s/kodofs.yaml
	kubectl delete -f $<

.PHONY: install_plugins
install_plugins: install_kodo_csi_driver install_kodofs_csi_driver

.PHONY: delete_plugins
delete_plugins: delete_kodo_csi_driver delete_kodofs_csi_driver
