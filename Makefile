.PHONY: \
	all \
	deps \
	update-deps \
	test-deps \
	update-test-deps \
	build \
	install \
	clean \
	shell \
	launch-shard \
	launch-pfsd \
	build-images \
	push-images \
	lint \
	vet \
	errcheck \
	pretest \
	test-long \
	test \
	test-new \
	bench \
	proto \
	hit-godoc

include etc/env/env.env

IMAGES = deploy pfsd ppsd router shard
BINARIES = deploy pfs pfsd pps ppsd router shard

all: test

print-%:
	@echo $* = $($*)

deps:
	go get -d -v ./...

update-deps:
	go get -d -v -u -f ./...

test-deps:
	go get -d -v -t ./...

update-test-deps:
	go get -d -v -t -u -f ./...

build: deps
	go build ./...

install: deps
	go install ./...

clean:
	go clean ./...
	bin/clean
	$(foreach image,$(IMAGES),PACHYDERM_IMAGE=$(image) bin/clean || exit;)
	$(foreach binary,$(BINARIES),rm -f src/cmd/$(binary)/$(binary);)
	sudo rm -rf _tmp

build-images:
	$(foreach image,$(IMAGES),PACHYDERM_IMAGE=$(image) bin/build || exit;)

push-images: build-images
	$(foreach image,$(IMAGES),docker push pachyderm/$(image) || exit;)

shell:
	PACHYDERM_DOCKER_OPTS="-it -v $(shell pwd):/go/src/github.com/pachyderm/pachyderm" bin/run /bin/bash

launch-shard:
	PACHYDERM_IMAGE=shard PACHYDERM_DOCKER_OPTS="-d" bin/run -shard 0 -modulos 1

launch-pfsd:
	PACHYDERM_IMAGE=pfsd PACHYDERM_DOCKER_OPTS="-d -p $(PFS_PORT):$(PFS_API_PORT) -p $(PFS_TRACE_PORT):$(PFS_TRACE_PORT)" bin/run

launch-ppsd:
	PACHYDERM_IMAGE=ppsd PACHYDERM_DOCKER_OPTS="-d -p $(PPS_PORT):$(PPS_API_PORT) -p $(PPS_TRACE_PORT):$(PPS_TRACE_PORT)" bin/run

kube-%:
	kubectl=kubectl; \
	if ! which $$kubectl > /dev/null; then \
		kubectl=kubectl.sh; \
		if ! which $$kubectl > /dev/null; then \
			echo "error: kubectl not installed" >& 2; \
			exit 1; \
		fi; \
	fi; \
	for file in storage-controller.yml router-controller.yml pachyderm-service.yml; do \
		$$kubectl $* -f etc/kube/$$file; \
	done

lint:
	go get -v github.com/golang/lint/golint
	golint ./...

vet:
	go vet ./...

errcheck:
	errcheck ./...

pretest: lint vet errcheck

# TODO(pedge): add pretest when fixed
test:
	bin/run bin/wrap bin/test -test.short ./...

# TODO(pedge): add pretest when fixed
test-long:
	@ echo WARNING: this will not work as an OSS contributor for now, we are working on fixing this.
	@ echo This directive requires Pachyderm AWS credentials. Sleeping for 5 seconds so you can ctrl+c if you want...
	@ sleep 5
	bin/run bin/wrap bin/test ./...

test-new: test-deps
	go get -v github.com/golang/lint/golint
	for pkg in pfs pkg pps; do \
		for file in $$(find "./src/$$pkg" -name '*.go' | grep -v '\.pb\.go'); do \
			golint $$file; \
		done; \
	done
	go vet ./src/pfs/... ./src/pkg/... ./src/pps/...
	bin/run bin/wrap bin/test ./src/pfs/... ./src/pps/...

# TODO(pedge): add pretest when fixed
bench:
	@ echo WARNING: this will not work as an OSS contributor for now, we are working on fixing this.
	@ echo This directive requires Pachyderm AWS credentials. Sleeping for 5 seconds so you can ctrl+c if you want...
	@ sleep 5
	bin/run bin/wrap bin/test -bench . ./...

proto:
	@ if ! docker images | grep 'pedge/proto3grpc' > /dev/null; then \
		docker pull pedge/proto3grpc; \
		fi
	docker run \
		--volume $(shell pwd):/compile \
		--workdir /compile \
		pedge/proto3grpc \
		protoc \
		-I /usr/include \
		-I /compile/src/pfs \
		--go_out=plugins=grpc,Mgoogle/protobuf/empty.proto=github.com/peter-edge/go-google-protobuf,Mgoogle/protobuf/timestamp.proto=github.com/peter-edge/go-google-protobuf,Mgoogle/protobuf/wrappers.proto=github.com/peter-edge/go-google-protobuf:/compile/src/pfs \
		/compile/src/pfs/pfs.proto
	docker run \
		--volume $(shell pwd):/compile \
		--workdir /compile \
		pedge/proto3grpc \
		protoc \
		-I /compile/src/pps \
		--go_out=plugins=grpc,Mgoogle/protobuf/empty.proto=github.com/peter-edge/go-google-protobuf,Mgoogle/protobuf/timestamp.proto=github.com/peter-edge/go-google-protobuf:/compile/src/pps \
		/compile/src/pps/pps.proto

hit-godoc:
	for pkg in $$(find . -name '*.go' | xargs dirname | sort | uniq); do \
		curl https://godoc.org/github.com/pachyderm/pachyderm/$pkg > /dev/null; \
	done
