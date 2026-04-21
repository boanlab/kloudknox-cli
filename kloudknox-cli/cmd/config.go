// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"errors"
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

const (
	defaultServer = "localhost:36890"
	defaultNS     = "kloudknox"
)

var globalCfg globalConfig

type globalConfig struct {
	Env         string
	KubeConfig  string
	KubeContext string
	Namespace   string
	Output      string
	Server      string
}

type clientConfig struct {
	Server string `json:"server,omitempty" yaml:"server,omitempty"`
}

// loadClientConfig resolves the gRPC endpoint. Priority (high→low):
// --server flag, ~/.kkctl.yaml, built-in default.
func loadClientConfig() clientConfig {
	cfg := clientConfig{Server: defaultServer}

	if home, err := os.UserHomeDir(); err == nil {
		if data, err := os.ReadFile(home + "/.kkctl.yaml"); err == nil { // #nosec G304
			var fileCfg clientConfig
			if yaml.Unmarshal(data, &fileCfg) == nil && fileCfg.Server != "" {
				cfg.Server = fileCfg.Server
			}
		}
	}

	if globalCfg.Server != "" {
		cfg.Server = globalCfg.Server
	}
	return cfg
}

func resolvedEnv() (string, error) {
	if globalCfg.Env != "" {
		e := globalCfg.Env
		if e != "k8s" && e != "docker" {
			return "", fmt.Errorf("--env must be k8s or docker, got %q", e)
		}
		return e, nil
	}
	return detectEnv()
}

func detectEnv() (string, error) {
	hasKube := false
	if home, err := os.UserHomeDir(); err == nil {
		if _, err := os.Stat(home + "/.kube/config"); err == nil {
			hasKube = true
		}
	}
	if _, err := os.Stat("/etc/kubernetes/admin.conf"); err == nil {
		hasKube = true
	}

	hasDocker := false
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		hasDocker = true
	}

	switch {
	case hasKube && hasDocker:
		return "", errors.New("both kubeconfig and docker.sock detected — specify --env k8s|docker")
	case hasKube:
		return "k8s", nil
	case hasDocker:
		return "docker", nil
	default:
		return "", errors.New("no kubernetes or docker environment detected — specify --env k8s|docker")
	}
}

func resolvedOutput() string {
	if globalCfg.Output == "" {
		return "table"
	}
	return globalCfg.Output
}

func resolvedNamespace() string {
	if globalCfg.Namespace == "" {
		return defaultNS
	}
	return globalCfg.Namespace
}
