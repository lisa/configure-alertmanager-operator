SHELL := /usr/bin/env bash -x

# Include shared Makefiles
include project.mk
include standard.mk
include functions.mk

default: gobuild

# Extend Makefile after here

# Build the docker image
.PHONY: docker-build
docker-build:
	$(MAKE) build

# Push the docker image
.PHONY: docker-push
docker-push:
	$(MAKE) push
