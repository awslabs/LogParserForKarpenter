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

VERSION?=v1.0

PKG=https://github.com/youwalther65/KarpenterLogParser
GIT_COMMIT?=$(shell git rev-parse HEAD)
BUILD_DATE?=$(shell date -u -Iseconds)
#LDFLAGS?="-X ${PKG}/pkg/driver.driverVersion=${VERSION} -X ${PKG}/pkg/driver.gitCommit=${GIT_COMMIT} -X ${PKG}/pkg/driver.buildDate=${BUILD_DATE} -s -w"
LDFLAGS?="-s -w"

OS?=$(shell go env GOHOSTOS)
ARCH?=$(shell go env GOHOSTARCH)

BINARY=lp4k

GO_SOURCES=go.mod go.sum $(shell find . -type f -name "*.go")

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


