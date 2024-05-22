DOCKER_BUILD_ARGUMENT := $(if $(DOCKER_BUILD_ARGUMENT),$(DOCKER_BUILD_ARGUMENT),build)
MAKEFLAGS += --always-make ### PHONY all make targets

image-generate:
	docker $(DOCKER_BUILD_ARGUMENT) -f build/image/generate/Dockerfile -t everoute/runtime/generate ./build/image/generate/

generate:
	find . -name "*.go" -exec gci write --Section Standard --Section Default --Section "Prefix(github.com/everoute/runtime)" {} +

container-shell: image-generate
	$(eval WORKDIR := /go/src/github.com/everoute/runtime)
	docker run --rm -it --security-opt seccomp=unconfined -w $(WORKDIR) -v $(CURDIR):$(WORKDIR) everoute/runtime/generate bash

docker-generate: image-generate
	$(eval WORKDIR := /go/src/github.com/everoute/runtime)
	docker run --rm -iu $$(id -u):$$(id -g) -w $(WORKDIR) -v $(CURDIR):$(WORKDIR) everoute/runtime/generate make generate

test:
	go test --gcflags=all=-l ./... --race --coverprofile coverage.out

docker-test:
	$(eval WORKDIR := /go/src/github.com/everoute/runtime)
	docker run --rm -iu 0:0 -w $(WORKDIR) -v $(CURDIR):$(WORKDIR) golang:1.20 make test
