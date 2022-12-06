# Copyright 2021 The Kubernetes Authors.
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


# Explicitly opt into go modules, even though we're inside a GOPATH directory
export GO111MODULE=on

# Image URL to use all building/pushing image targets
DOCKER_REG ?= ${or ${DOCKER_REGISTRY},"guofei.azurecr.io"}
IMG ?= ${DOCKER_REG}/vk-benchmark-amd64 
TAG ?= 0.0.1

# TEST_FLAGS used as flags of go test.
TEST_FLAGS ?= -v

export KUBEBUILDER_ASSETS=/tmp/kubebuilder/bin/

.PHONY: all
all: build

build:
	go build -o _output/bin/vk-benchmark ./cmd/benchmark/	

build-image:
	docker build -t ${DOCKER_REG}/vk-benchmark:${TAG} .

push: build-image
	docker push ${DOCKER_REG}/vk-benchmark:${TAG}


