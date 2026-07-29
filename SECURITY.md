# Security Policy

## Supported Versions

| Version | Supported |
|---|---|
| v0.1.3 (latest) | Yes |
| Older commits / forks | No |

Only the latest release and the tip of `main` receive security fixes.

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Please report security issues by email to **namjh@dankook.ac.kr** with the subject line `[KloudKnox Security]`.

Include:

- A description of the vulnerability and its potential impact
- Steps to reproduce or a proof-of-concept
- Your environment (OS, kernel version, Kubernetes version, commit SHA of KloudKnox)
- Affected component (agent, operator, webhook, relay, CLI)
- Any suggested mitigations if you have them

We aim to acknowledge reports within 5 business days.

## Disclosure Policy

We follow a coordinated disclosure model. Please allow us reasonable time to address the vulnerability before any public disclosure. We will credit reporters in the release notes unless you prefer to remain anonymous.

## Scope

This repository covers the `kkctl` command-line client. For vulnerabilities in other components, report under the corresponding repository:

- Agent / eBPF / enforcer / monitor: https://github.com/boanlab/KloudKnox
- Operator controller: https://github.com/boanlab/KloudKnox (subdirectory `operator-controller/`)
- gRPC schema (protobuf): https://github.com/boanlab/KloudKnox (subdirectory `protobuf/`)
- AppArmor webhook: https://github.com/boanlab/kloudknox-apparmor-webhook
- Relay server: https://github.com/boanlab/kloudknox-relay-server

The following are out of scope:

- Third-party dependencies (report to the upstream project)
- Misconfigurations in user-supplied policies or cluster RBAC
