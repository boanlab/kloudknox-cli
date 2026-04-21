// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import _ "embed"

//go:embed embed/k8s/00_kloudknox_namespace.yaml
var manifestNamespace []byte

//go:embed embed/k8s/01_kloudknoxpolicy.yaml
var manifestCRD []byte

//go:embed embed/k8s/02_operator-controller.yaml
var manifestOperator []byte

//go:embed embed/k8s/03_apparmor_webhook.yaml
var manifestWebhook []byte

//go:embed embed/k8s/04_kloudknox.yaml
var manifestDaemonSet []byte

//go:embed embed/k8s/99_relay-server.yaml
var manifestRelay []byte

//go:embed embed/docker/docker-compose.yaml
var dockerComposeYAML []byte

//go:embed embed/completion/bash.sh
var completionBash []byte

//go:embed embed/completion/zsh.sh
var completionZsh []byte

// k8sManifests returns the embedded manifests in install order. Namespace
// first, CRD before any CR, workloads last so the API server is ready.
func k8sManifests() [][]byte {
	return [][]byte{
		manifestNamespace,
		manifestCRD,
		manifestOperator,
		manifestWebhook,
		manifestDaemonSet,
		manifestRelay,
	}
}

// k8sManifestsFor returns manifests in install order. Pass skipWebhook=true on
// BPF-LSM-only clusters where the AppArmor mutation webhook is not needed.
func k8sManifestsFor(skipWebhook bool) [][]byte {
	all := k8sManifests()
	if !skipWebhook {
		return all
	}
	// manifestWebhook is index 3; drop it while preserving the rest.
	return append(all[:3:3], all[4:]...)
}
