IMAGE := adiecho/oci-panel
TAG   := latest

.PHONY: build push push-amd64 push-arm64 clean

## 构建多架构镜像并推送到 Docker Hub（不依赖 buildx docker-container driver）
push: push-multi

push-multi:
	docker build --platform linux/amd64 -t $(IMAGE):$(TAG)-amd64 .
	docker build --platform linux/arm64 -t $(IMAGE):$(TAG)-arm64 .
	docker push $(IMAGE):$(TAG)-amd64
	docker push $(IMAGE):$(TAG)-arm64
	docker manifest rm $(IMAGE):$(TAG) 2>/dev/null || true
	docker manifest create $(IMAGE):$(TAG) \
		$(IMAGE):$(TAG)-amd64 \
		$(IMAGE):$(TAG)-arm64
	docker manifest push $(IMAGE):$(TAG)

## 仅构建并推送 amd64
push-amd64:
	docker build --platform linux/amd64 -t $(IMAGE):$(TAG) .
	docker push $(IMAGE):$(TAG)

## 仅构建并推送 arm64
push-arm64:
	docker build --platform linux/arm64 -t $(IMAGE):$(TAG) .
	docker push $(IMAGE):$(TAG)

## 构建到本地（当前架构）
build:
	docker build -t $(IMAGE):$(TAG) .

## 清理临时架构标签
clean:
	docker rmi $(IMAGE):$(TAG)-amd64 $(IMAGE):$(TAG)-arm64 2>/dev/null || true
