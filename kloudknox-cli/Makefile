# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 BoanLab @ Dankook University

PROG_NAME  = kkctl
IMAGE_NAME = boanlab/kloudknox-cli
TAG        ?= v0.1.0

LDFLAGS = -s -w -X github.com/boanlab/kloudknox-cli/cmd.Version=$(TAG)

GO_FILES = $(shell find . -type f -name '*.go' -not -path './vendor/*')

## RELEASE_PLATFORMS lists the os/arch pairs published as release artifacts.
## Windows is excluded: `kkctl probe` reads uname and /proc through
## golang.org/x/sys/unix, which has no Windows implementation.
RELEASE_PLATFORMS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64
DIST_DIR          ?= dist

## KK_REF selects which branch/tag of the boanlab/KloudKnox family of repos the
## embedded deployment manifests are fetched from. Defaults to `main` for
## bleeding-edge; pin to a tag (e.g. KK_REF=v0.1.0) for reproducible builds.
KK_REF ?= main
KK_RAW         = https://raw.githubusercontent.com/boanlab/KloudKnox/$(KK_REF)/deployments
KK_WEBHOOK_RAW = https://raw.githubusercontent.com/boanlab/kloudknox-apparmor-webhook/$(KK_REF)/deployments
KK_RELAY_RAW   = https://raw.githubusercontent.com/boanlab/kloudknox-relay-server/$(KK_REF)/deployments

.PHONY: all build test gofmt vet golangci-lint gosec clean \
        install-kkctl build-image push-image sync-manifests \
        release-binaries \
        run-events run-alerts run-logs

all: build

## build: vet + format check + build the binary
build: gofmt vet
	go mod tidy
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/$(PROG_NAME)

## test: run the full unit-test suite
test:
	go test ./... -v -count=1 -race

## gofmt: rewrite files to canonical gofmt output
gofmt:
	gofmt -s -w $(GO_FILES)

## vet: run `go vet`
vet:
	go vet ./...

## golangci-lint: run golangci-lint (installs it on demand)
golangci-lint:
ifeq (, $(shell which golangci-lint))
	@{ \
	set -e ;\
	tmp=$$(mktemp -d) ;\
	cd $$tmp ;\
	go mod init tmp ;\
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest ;\
	rm -rf $$tmp ;\
	}
endif
	golangci-lint run

## gosec: run gosec static analysis (installs it on demand)
gosec:
ifeq (, $(shell which gosec))
	@{ \
	set -e ;\
	tmp=$$(mktemp -d) ;\
	cd $$tmp ;\
	go mod init tmp ;\
	go install github.com/securego/gosec/v2/cmd/gosec@latest ;\
	rm -rf $$tmp ;\
	}
endif
	gosec ./...

## install-kkctl: install $(PROG_NAME) into $(GOBIN) (or $HOME/go/bin)
install-kkctl: build
	install -D -m 0755 bin/$(PROG_NAME) $${GOBIN:-$$HOME/go/bin}/$(PROG_NAME)

run-events:
	bin/$(PROG_NAME) stream events

run-alerts:
	bin/$(PROG_NAME) stream alerts

run-logs:
	bin/$(PROG_NAME) stream logs

clean:
	rm -rf bin $(DIST_DIR)

## release-binaries: build a static binary per RELEASE_PLATFORMS into $(DIST_DIR),
## each as a .tar.gz alongside a single checksum file. CGO is disabled so the
## binaries carry no runtime dependency and run on any distro of that os/arch.
## Pass TAG to stamp the version, e.g. make release-binaries TAG=v0.1.0
release-binaries:
	rm -rf $(DIST_DIR)
	mkdir -p $(DIST_DIR)
	@for platform in $(RELEASE_PLATFORMS); do \
	    os=$${platform%%/*}; arch=$${platform##*/}; \
	    echo "==> $(PROG_NAME) $(TAG) $$os/$$arch"; \
	    CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
	        go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST_DIR)/$(PROG_NAME) . || exit 1; \
	    tar -C $(DIST_DIR) -czf \
	        $(DIST_DIR)/$(PROG_NAME)_$(TAG)_$${os}_$${arch}.tar.gz $(PROG_NAME) || exit 1; \
	    rm -f $(DIST_DIR)/$(PROG_NAME); \
	done
	cd $(DIST_DIR) && sha256sum *.tar.gz > $(PROG_NAME)_$(TAG)_checksums.txt
	@echo "==> artifacts in $(DIST_DIR)/"

build-image:
	docker build -f Dockerfile -t $(IMAGE_NAME):$(TAG) -t $(IMAGE_NAME):latest \
		--build-arg VERSION=$(TAG) .

push-image: build-image
	docker push $(IMAGE_NAME):$(TAG)
	docker push $(IMAGE_NAME):latest

## sync-manifests: refresh embedded deployment YAMLs. The core manifests come
## from boanlab/KloudKnox at $(KK_REF); the apparmor-webhook and relay-server
## manifests come from their own split repos at the same ref. Run this before
## cutting a release so `kkctl install` reflects the authoritative manifests.
sync-manifests:
	@mkdir -p cmd/embed/k8s
	@for f in 00_kloudknox_namespace.yaml 01_kloudknoxpolicy.yaml 02_operator-controller.yaml 04_kloudknox.yaml; do \
		echo "fetching $$f from boanlab/KloudKnox@$(KK_REF)"; \
		curl -fsSL -o cmd/embed/k8s/$$f $(KK_RAW)/$$f; \
	done
	@echo "fetching 03_apparmor_webhook.yaml from boanlab/kloudknox-apparmor-webhook@$(KK_REF)"
	@curl -fsSL -o cmd/embed/k8s/03_apparmor_webhook.yaml $(KK_WEBHOOK_RAW)/webhook.yaml
	@echo "fetching 99_relay-server.yaml from boanlab/kloudknox-relay-server@$(KK_REF)"
	@curl -fsSL -o cmd/embed/k8s/99_relay-server.yaml $(KK_RELAY_RAW)/relay-server.yaml
