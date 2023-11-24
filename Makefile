include common.mk

.PHONY: all
all: build

.PHONY: build
build: connector/$(CONNECTOR_FILENAME) plugin/$(PLUGIN_FILENAME)

connector/$(CONNECTOR_FILENAME):
	cd connector && \
		CGO_ENABLED=0 go build -ldflags \
		"-X main.VERSION=$(VERSION) -X main.COMMITID=$(COMMIT_ID) -X main.BUILDTIME=$(BUILD_TIME)" \
		-o $(CONNECTOR_FILENAME)

plugin/$(PLUGIN_FILENAME):
	cd plugin && \
		CGO_ENABLED=0 go build -ldflags \
		"-X main.VERSION=$(VERSION) -X main.COMMITID=$(COMMIT_ID) -X main.BUILDTIME=$(BUILD_TIME)" \
		-o $(PLUGIN_FILENAME)

.PHONY: clean
clean:
	rm -f connector/$(CONNECTOR_FILENAME) \
		plugin/$(PLUGIN_FILENAME)
	rm -f k8s/kodo.yaml k8s/kodofs.yaml
	rm -f docker/rclone docker/kodofs

k8s/kodo.yaml: k8s/kodo/kodo-plugin.yaml k8s/kodo/kodo-rbac.yaml k8s/kodo/kodo-provisioner.yaml common.mk
	@cat k8s/kodo/kodo-plugin.yaml \
		| sed 's/$${DOCKERHUB_ORGANIZATION}/$(subst /,\/,$(DOCKERHUB_ORGANIZATION))/g' \
		| sed 's/$${DOCKERHUB_IMAGE}/$(DOCKERHUB_IMAGE)/g' \
		| sed 's/$${DOCKERHUB_TAG}/$(DOCKERHUB_TAG)/g' \
		>> k8s/kodo.yaml
	@echo --- >> k8s/kodo.yaml
	@cat k8s/kodo/kodo-rbac.yaml >> k8s/kodo.yaml
	@echo --- >> k8s/kodo.yaml
	@cat k8s/kodo/kodo-provisioner.yaml >> k8s/kodo.yaml

k8s/kodofs.yaml: k8s/kodofs/kodofs-plugin.yaml k8s/kodofs/kodofs-rbac.yaml k8s/kodofs/kodofs-provisioner.yaml common.mk
	@cat k8s/kodofs/kodofs-plugin.yaml \
		| sed 's/$${DOCKERHUB_ORGANIZATION}/$(subst /,\/,$(DOCKERHUB_ORGANIZATION))/g' \
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

.PHONY: docker/rclone
docker/rclone:
	curl -LJO# https://github.com/rclone/rclone/releases/download/$(RCLONE_VERSION)/rclone-$(RCLONE_VERSION)-linux-amd64.zip
	unzip rclone-$(RCLONE_VERSION)-linux-amd64.zip
	mv rclone-$(RCLONE_VERSION)-linux-amd64/rclone docker/rclone
	chmod +x docker/rclone
	rm rclone-$(RCLONE_VERSION)-linux-amd64.zip
	rm -rf rclone-$(RCLONE_VERSION)-linux-amd64

# 下载kodofs二进制文件，由于kodofs是私有仓库，所以需要携带 Github API Token 才能下载
.PHONY: docker/kodofs
docker/kodofs:
	@if [ -z $$GITHUB_API_TOKEN ];\
		then \
			echo "Please configure environment GITHUB_API_TOKEN"; \
			exit 1; \
	fi
	cd scripts && bash get_gh_asset.sh qbox kodofs $(KODOFS_VERSION) kodofs
	mv scripts/kodofs docker/kodofs
	chmod +x docker/kodofs

.PHONY: build_image
build_image: docker/rclone docker/kodofs
	docker build --pull \
		-t="$(DOCKERHUB_ORGANIZATION)/$(DOCKERHUB_IMAGE):$(VERSION)" \
		-f Dockerfile .

.PHONY: push_image
push_image: build_image
	docker push "$(DOCKERHUB_ORGANIZATION)/$(DOCKERHUB_IMAGE):$(VERSION)"

.PHONY: install_kodo_static_example
install_kodo_static_example: k8s/kodo.yaml
	kubectl apply -f k8s/kodo.yaml
	kubectl apply -f examples/kodo/static-provisioning/
	kubectl apply -f examples/kodo/deploy.yaml

.PHONY: delete_kodo_static_example
delete_kodo_static_example:
	kubectl delete -f examples/kodo/deploy.yaml || true
	kubectl delete -f examples/kodo/static-provisioning/ || true
	kubectl delete -f k8s/kodo.yaml || true

.PHONY: install_kodo_dynamic_example
install_kodo_dynamic_example: k8s/kodo.yaml
	kubectl apply -f k8s/kodo.yaml
	kubectl apply -f examples/kodo/dynamic-provisioning/
	kubectl apply -f examples/kodo/deploy.yaml

.PHONY: delete_kodo_dynamic_example
delete_kodo_dynamic_example:
	kubectl delete -f examples/kodo/deploy.yaml || true
	kubectl delete -f examples/kodo/dynamic-provisioning/ || true
	kubectl delete -f k8s/kodo.yaml || true

.PHONY: install_kodofs_static_example
install_kodofs_static_example: k8s/kodofs.yaml
	kubectl apply -f k8s/kodofs.yaml
	kubectl apply -f examples/kodofs/static-provisioning/
	kubectl apply -f examples/kodofs/deploy.yaml

.PHONY: delete_kodofs_static_example
delete_kodofs_static_example:
	kubectl delete -f examples/kodofs/deploy.yaml || true
	kubectl delete -f examples/kodofs/static-provisioning/ || true
	kubectl delete -f k8s/kodofs.yaml || true

.PHONY: install_kodofs_dynamic_example
install_kodofs_dynamic_example: k8s/kodofs.yaml
	kubectl apply -f k8s/kodofs.yaml
	kubectl apply -f examples/kodofs/dynamic-provisioning/
	kubectl apply -f examples/kodofs/deploy.yaml

.PHONY: delete_kodofs_dynamic_example
delete_kodofs_dynamic_example:
	kubectl delete -f examples/kodofs/deploy.yaml || true
	kubectl delete -f examples/kodofs/dynamic-provisioning/ || true
	kubectl delete -f k8s/kodofs.yaml || true
