# Contribution Guide

This guide describes how to set up a development environment for `kloudknox-cli` and submit changes.

## Prerequisites

- Go 1.24+
- Docker (for building container images)
- `kubectl` and access to a Kubernetes cluster (for testing `kkctl install`, `kkctl stream`, and other in-cluster flows)
- `golangci-lint` and `gosec` (auto-installed by `make` if missing)
- `curl` (used by `make sync-manifests` to fetch the embedded KloudKnox manifests)

## Development Workflow

All source code lives under the `kloudknox-cli/` subdirectory.

```bash
cd kloudknox-cli

# Build
make build           # runs gofmt, vet, then compiles bin/kkctl

# Run each check individually
make gofmt
make vet
make golangci-lint
make gosec
make test
```

## Embedded Deployment Manifests

`kkctl install` embeds the KloudKnox deployment manifests via `go:embed`. The committed copies under `kloudknox-cli/cmd/embed/k8s/` are snapshots of the authoritative YAML in the [boanlab/KloudKnox](https://github.com/boanlab/KloudKnox) repo under `deployments/`. Refresh them before cutting a release:

```bash
cd kloudknox-cli
make sync-manifests KK_REF=main       # track bleeding edge
make sync-manifests KK_REF=v0.1.0     # pin to a tag (preferred for releases)
```

Commit the refreshed YAML alongside your changes.

## Container Image

```bash
cd kloudknox-cli
make build-image TAG=v0.1.0
make push-image  TAG=v0.1.0   # requires registry auth
```

## Submitting Changes

1. Fork and create a feature branch (`feature/<topic>` or `fix/<topic>`).
2. Run `make build` and `make test` from `kloudknox-cli/` before pushing.
3. If you refreshed embedded manifests, commit the regenerated files under `cmd/embed/k8s/`.
4. Open a pull request against `main`. Link related issues.
5. CI runs `gofmt`, `gosec`, unit tests, and a Docker image build.

## Commit Message Convention

```
<type>(<scope>): <subject>

<body>
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`.

---

Copyright 2026 [BoanLab](https://boanlab.com) @ Dankook University
