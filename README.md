# kloudknox-cli

`kkctl` is the command-line client for [KloudKnox](https://github.com/boanlab/KloudKnox). Use it to install and operate the agent, manage policies, and tail security events, alerts, and logs in real time.

`kkctl` targets both **Kubernetes** and **Docker**: it detects the environment automatically and adapts each command accordingly. Pass `--env k8s` or `--env docker` to force a mode. The one exception is `describe container`, which always queries the Docker socket.

For writing policies, see the [policy authoring guide](https://github.com/boanlab/KloudKnox/blob/main/getting-started/policy-authoring.md). For Docker-mode deployment, see the [docker-mode guide](https://github.com/boanlab/KloudKnox/blob/main/getting-started/docker-mode.md). For recipes, see [use-cases](https://github.com/boanlab/KloudKnox/blob/main/getting-started/use-cases.md).

## Requirements

- Linux host with a running KloudKnox agent (or permission to install one)
- For building from source: Go 1.24 or newer
- For `kkctl install` in Kubernetes mode on nodes that rely on AppArmor: [cert-manager](https://cert-manager.io/) in the cluster (used by the bundled AppArmor mutation webhook). BPF-LSM-only clusters can pass `--skip-apparmor-webhook` to drop this dependency.

## Install

### Pre-built binary

Every `v*` tag publishes a `kkctl` binary per platform, plus a `sha256` checksum
file, as GitHub release assets. The binaries carry no runtime dependency — Linux
builds are fully static, macOS builds are CGO-free and link only libSystem.

| OS | Architectures |
|---|---|
| linux | `amd64`, `arm64` |
| darwin | `amd64`, `arm64` |

Windows is not published: `kkctl probe` reads uname and `/proc` through
`golang.org/x/sys/unix`, which has no Windows implementation.

```bash
VERSION=v0.1.0
OS=$(uname -s | tr '[:upper:]' '[:lower:]')      # linux | darwin
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

BASE=https://github.com/boanlab/kloudknox-cli/releases/download/${VERSION}
curl -fsSLO ${BASE}/kkctl_${VERSION}_${OS}_${ARCH}.tar.gz
curl -fsSLO ${BASE}/kkctl_${VERSION}_checksums.txt
sha256sum --ignore-missing -c kkctl_${VERSION}_checksums.txt

tar -xzf kkctl_${VERSION}_${OS}_${ARCH}.tar.gz
sudo install -m 0755 kkctl /usr/local/bin/kkctl
kkctl version
```

> While the repository is private, `curl` cannot reach the asset. Use the
> authenticated GitHub CLI instead:
> `gh release download ${VERSION} --repo boanlab/kloudknox-cli --pattern 'kkctl_*'`

### From source

```bash
git clone https://github.com/boanlab/kloudknox-cli.git
cd kloudknox-cli/kloudknox-cli
make                 # builds bin/kkctl
make install-kkctl   # installs kkctl to $GOBIN (or $HOME/go/bin)
```

To reproduce the published artifacts locally:

```bash
make release-binaries TAG=v0.1.0   # writes dist/*.tar.gz + checksums
```

> **Note:** `make install-kkctl` writes `kkctl` to `$GOBIN` (or `$HOME/go/bin`), which is not on `sudo`'s `secure_path`. Install into a system path instead:
>
> ```bash
> sudo install -m 0755 bin/kkctl /usr/local/bin/kkctl
> ```
>
> Or preserve `$PATH` per call: `sudo -E env PATH=$PATH kkctl probe`.

The `kkctl install` command embeds KloudKnox deployment manifests into the binary via `go:embed`. The committed copies under `kloudknox-cli/cmd/embed/k8s/` are snapshots of the authoritative YAML in the [boanlab/KloudKnox](https://github.com/boanlab/KloudKnox) repo under `deployments/`. Refresh them from a specific ref with:

```bash
cd kloudknox-cli
make sync-manifests KK_REF=main     # or KK_REF=v0.1.0
```

Run this whenever the upstream manifests change — in particular the
`KloudKnoxPolicy` CRD. A stale embedded CRD makes `kkctl install` provision a
schema that rejects newer policy fields with
`strict decoding error: unknown field "spec.<field>"`, even though the same
policy applies cleanly from `deployments/`.

### Container image

```bash
cd kloudknox-cli
make build-image TAG=v0.1.0
make push-image  TAG=v0.1.0
```

### In-cluster RBAC

To run `kkctl` from inside a Kubernetes cluster (for example, as a debug pod), apply the bundled RBAC and Deployment manifest:

```bash
kubectl apply -f deployments/kloudknox-cli.yaml

# Then exec into the pod to run kkctl
kubectl exec -it -n kloudknox deploy/kloudknox-cli -- kkctl get policies
```

## Quick start

```bash
# 1) Check that the host is ready to run KloudKnox
sudo kkctl probe

# 2) Install KloudKnox (K8s or Docker is chosen automatically)
sudo kkctl install

# 3) Confirm the agent is healthy
kkctl status

# 4) Apply a policy and watch alerts
kkctl apply -f ./policies/nginx-hardening.yaml
kkctl stream alerts
```

## Commands

### Lifecycle

| Command | Description |
|---|---|
| `kkctl probe` | Check kernel, BTF, cgroup v2, capabilities, and enforcer mode (BPF LSM vs AppArmor) |
| `kkctl install [--image <img>] [--wait=true] [--skip-apparmor-webhook]` | Install the KloudKnox agent (pass `--skip-apparmor-webhook` on BPF-LSM-only clusters) |
| `kkctl upgrade --image <img> [--dry-run]` | Roll the agent to a new image |
| `kkctl uninstall [--purge-policies]` | Remove the agent (optionally purge all policies) |
| `kkctl status [-o table\|json]` | Show agent and node status |

### Policies

| Command | Description |
|---|---|
| `kkctl apply -f <file> [--dry-run]` | Create or update one or more policies |
| `kkctl delete -f <file>` | Delete the policies declared in a YAML file |
| `kkctl delete policy <name>...` | Delete policies by name |
| `kkctl get policies [-A] [-o table\|wide\|json\|yaml]` | List policies |
| `kkctl get nodes [-o table\|json]` | List nodes where KloudKnox is running |
| `kkctl describe policy\|node <name>` | Show full details for a resource |
| `kkctl describe container <name>` | Show container details — **Docker mode only** (queries the Docker socket regardless of `--env`) |
| `kkctl policy validate -f <file>` | Validate a policy YAML offline |

### Labels and selectors

| Command | Description |
|---|---|
| `kkctl label container\|pod <name> <k=v> [k=v ...]` | Add labels to a container or pod |
| `kkctl inject -f <policy> --target container\|pod --name <n>` | Copy a policy's `matchLabels` onto the target |

### Live streams

KloudKnox emits one JSON object per line (NDJSON). Reconnection is automatic.

| Command | Description |
|---|---|
| `kkctl stream events [filter-flags]` | System events for policy-matched activity |
| `kkctl stream alerts [filter-flags]` | Policy-triggered alerts |
| `kkctl stream logs [--level INFO\|WARN\|ERROR]` | Agent logs |

`stream events` only reports pods selected by a `KloudKnoxPolicy` — monitoring is
gated in the kernel, so with no policy applied the stream stays connected and
prints nothing. For a pod under a policy, the agent default `-visibility=policy`
emits only rule-matched events; `-visibility=full` also emits that pod's
unmatched events.

Filter flags (empty value = match all):
`--eventName`, `--source`, `--category`, `--operation`, `--resource`,
`--data`, `--nodeName`, `--namespaceName`, `--podName`, `--containerName`,
`--labels`.

### Diagnostics & utilities

| Command | Description |
|---|---|
| `kkctl sysdump [-o <file.tar.gz>]` | Collect a support bundle (probe, status, policies, logs) |
| `kkctl completion bash\|zsh` | Print shell completion script |
| `kkctl version` | Print the CLI version |

## Global flags

Place these **before** the verb, e.g. `kkctl --namespace prod get policies`.

| Flag | Default | Notes |
|---|---|---|
| `--env k8s\|docker` | auto-detect | Force the target environment |
| `--kubeconfig <path>` | `~/.kube/config` | Kubernetes modes only |
| `--kube-context <name>` | current context | Kubernetes modes only |
| `--namespace <ns>` | `kloudknox` | Kubernetes modes only |
| `--server <addr>` | `localhost:36890` | gRPC endpoint for `stream` |
| `-o <format>` | `table` | Output format |

## Configuration file

`kkctl` reads `~/.kkctl.yaml` if it exists. Command-line flags always win.

```yaml
server: localhost:36890
```

## Examples

```bash
# Apply a multi-document policy bundle
kkctl apply -f ./policies/bundle.yaml

# List every policy in every namespace with counts
kkctl get policies -A -o wide

# Stream alerts from production only
kkctl stream alerts --namespaceName=prod

# Watch file-category events from pods whose name starts with "web-"
kkctl stream events --category=File --podName=web-

# Collect a debug bundle to share with support
kkctl sysdump -o kloudknox-$(date +%F).tar.gz
```

## Project Structure

```
kloudknox-cli/
  kloudknox-cli/         # Go module source
    cmd/                 # Cobra command implementations
      embed/             # Embedded manifests and completion scripts
    main.go
    Makefile
    Dockerfile
  deployments/           # Kubernetes RBAC + Deployment manifest
  contribution/          # Development environment setup
```

## License

This project is licensed under the Apache License 2.0. See the [LICENSE](LICENSE) file for details.

---

Copyright 2026 [BoanLab](https://boanlab.com) @ Dankook University
