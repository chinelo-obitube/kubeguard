
SHELL := /bin/bash

CLUSTER_NAME ?= kubeguard
IMAGE ?= kubeguard:dev
NAMESPACE ?= kubeguard-system
LOCAL_PORT ?= 8081

.DEFAULT_GOAL := help

.PHONY: help test build cluster-up build-image load-image deploy \
        deploy-isolation-samples deploy-hardening-samples \
        verify-isolation verify-hardening port-forward status

verify-isolation:
	API_URL=http://localhost:$(LOCAL_PORT) bash scripts/verify-isolation.sh

verify-hardening: reset-hardening-samples
	API_URL=http://localhost:$(LOCAL_PORT) bash scripts/verify-hardening.sh

help:
	@echo "Kubeguard commands:"
	@echo "  make help                       Show available commands"
	@echo "  make test                       Run all Go tests"
	@echo "  make build                      Build the Go binary"
	@echo "  make cluster-up                 Create the kind cluster"
	@echo "  make build-image                Build the Docker image"
	@echo "  make load-image                 Load the image into kind"
	@echo "  make deploy                     Deploy Kubeguard"
	@echo "  make deploy-isolation-samples   Deploy network isolation demo workloads"
	@echo "  make deploy-hardening-samples   Deploy hardening demo workloads"
	@echo "  make port-forward               Expose Kubeguard on localhost"
	@echo "  make verify-isolation           Test bidirectional isolation and restoration"
	@echo "  make status                     Show deployment and Pod status"
	@echo "  make verify-hardening           Test multiple-namespace hardening"
	@echo "  make verify                     Run all end-to-end checks"
	@echo "  make reset-hardening-samples    Reset hardening demo workloads"
	@echo "  make install-calico             Install Calico CNI in kind"

test:
	go test ./...

build:
	go build -o kubeguard ./cmd/kubeguard

cluster-up:
	kind create cluster --name $(CLUSTER_NAME) --config kind-config.yaml

build-image:
	docker build -t $(IMAGE) .

load-image:
	kind load docker-image $(IMAGE) --name $(CLUSTER_NAME)

deploy:
	kubectl apply -f deploy/rbac.yaml
	kubectl apply -f deploy/kubeguard.yaml
	kubectl -n $(NAMESPACE) rollout status deployment/kubeguard --timeout=120s

deploy-isolation-samples:
	kubectl apply -f samples/isolation-demo.yaml

port-forward:
	kubectl -n $(NAMESPACE) port-forward svc/kubeguard $(LOCAL_PORT):8080

status:
	kubectl -n $(NAMESPACE) get deployments,pods,services
	kubectl get networkpolicies -A


deploy-hardening-samples:
	kubectl apply -f samples/hardening-demo.yaml
	kubectl -n hardening-demo-a rollout status deployment/demo-app --timeout=180s
	kubectl -n hardening-demo-b rollout status deployment/demo-app --timeout=180s

.PHONY: reset-hardening-samples

reset-hardening-samples:
	kubectl -n hardening-demo-a delete deployment demo-app --ignore-not-found --wait=true
	kubectl -n hardening-demo-b delete deployment demo-app --ignore-not-found --wait=true
	$(MAKE) deploy-hardening-samples

.PHONY: verify

verify: verify-isolation verify-hardening

CALICO_VERSION ?= v3.33.0

install-calico:
	kubectl apply -f https://raw.githubusercontent.com/projectcalico/calico/$(CALICO_VERSION)/manifests/tigera-operator.yaml
	kubectl wait --for=create crd/installations.operator.tigera.io --timeout=180s
	kubectl wait --for=condition=Established crd/installations.operator.tigera.io --timeout=180s
	kubectl apply -f deploy/calico-installation.yaml
	kubectl wait --for=condition=Ready installation/default --timeout=300s
	kubectl wait --for=condition=Ready nodes --all --timeout=300s