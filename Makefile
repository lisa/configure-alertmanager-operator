SHELL := /bin/bash
include version.mk

IMAGE_URI=quay.io/openshift-sre/configure-alertmanager-operator

VERSION_MAJOR=0
VERSION_MINOR=1

BINFILE=build/_output/bin/configure-alertmanager-operator
MAINPACKAGE=./cmd/manager
GOENV=GOOS=linux GOARCH=amd64 CGO_ENABLED=0
GOFLAGS=-gcflags="all=-trimpath=${GOPATH}" -asmflags="all=-trimpath=${GOPATH}"

.PHONY: all
all: check dockerbuild

.PHONY: check
check: ## Lint code
	gofmt -s -l $(shell go list -f '{{ .Dir }}' ./... ) | grep ".*\.go"; if [ "$$?" = "0" ]; then gofmt -s -d $(shell go list -f '{{ .Dir }}' ./... ); exit 1; fi
	go vet ./cmd/... ./pkg/...

.PHONY: dockerbuild
dockerbuild:
	docker build -f build/Dockerfile . -t $(IMAGE_URI):test20190405

# This part is done by the docker build
.PHONY: gobuild
gobuild: ## Build binary
	${GOENV} go build ${GOFLAGS} -o ${BINFILE} ${MAINPACKAGE}

