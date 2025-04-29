# Copyright 2023 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

## Variables/Functions

PKG=https://github.com/awslabs/KarpenterLogParser
GIT_COMMIT?=$(shell git rev-parse HEAD)
BUILD_DATE?=$(shell date -u -Iseconds)
LDFLAGS?="-s -w"

OS?=$(shell go env GOHOSTOS)
ARCH?=$(shell go env GOHOSTARCH)

BINARY=lp4k
TOOLS=lp4kcm

#GO_SOURCES=go.mod go.sum $(shell find . -type f -name "*.go")
GO_SOURCES=go.mod go.sum main.go ./k8s/k8s.go ./parser/parser.go
TOOLS_SOURCES=go.mod go.sum ./tools/lp4kcm.go ./k8s/k8s.go ./parser/parser.go

ALL_ARCH_linux?=amd64 arm64

.EXPORT_ALL_VARIABLES:

## Default target
# When no target is supplied, make runs the first target that does not begin with a .
# Alias that to building the binary
.PHONY: default
default: bin/$(BINARY)

## Top level targets

.PHONY: clean
clean:
	rm -rf bin/

## Builds

bin:
	@mkdir -p $@

bin/$(BINARY): $(GO_SOURCES) | bin
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build -mod=readonly -ldflags ${LDFLAGS} -o $@ .

.PHONY: all
all: bin/$(BINARY) bin/$(TOOLS)

.PHONY: tools
tools: bin/$(TOOLS)

bin/$(TOOLS): $(TOOLS_SOURCES) | bin
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build -mod=readonly -ldflags ${LDFLAGS} -o $@  ./tools/

.PHONY: install
install: bin/$(BINARY) bin/$(TOOLS)
	sudo cp bin/* /usr/local/bin

.PHONY: update
update:
	go mod tidy


