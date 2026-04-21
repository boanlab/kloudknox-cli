// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"testing"
)

func withGlobalCfg(fn func()) {
	saved := globalCfg
	defer func() { globalCfg = saved }()
	fn()
}

func TestResolvedOutput_Default(t *testing.T) {
	withGlobalCfg(func() {
		globalCfg.Output = "table"
		if got := resolvedOutput(); got != "table" {
			t.Errorf("resolvedOutput = %q, want table", got)
		}
	})
}

func TestResolvedOutput_JSON(t *testing.T) {
	withGlobalCfg(func() {
		globalCfg.Output = "json"
		if got := resolvedOutput(); got != "json" {
			t.Errorf("resolvedOutput = %q, want json", got)
		}
	})
}

func TestResolvedOutput_Empty(t *testing.T) {
	withGlobalCfg(func() {
		globalCfg.Output = ""
		if got := resolvedOutput(); got != "table" {
			t.Errorf("resolvedOutput (empty) = %q, want table", got)
		}
	})
}

func TestResolvedNamespace_Default(t *testing.T) {
	withGlobalCfg(func() {
		globalCfg.Namespace = ""
		if got := resolvedNamespace(); got != defaultNS {
			t.Errorf("resolvedNamespace = %q, want %q", got, defaultNS)
		}
	})
}

func TestResolvedNamespace_Custom(t *testing.T) {
	withGlobalCfg(func() {
		globalCfg.Namespace = "my-ns"
		if got := resolvedNamespace(); got != "my-ns" {
			t.Errorf("resolvedNamespace = %q, want my-ns", got)
		}
	})
}

func TestResolvedEnv_ExplicitK8s(t *testing.T) {
	withGlobalCfg(func() {
		globalCfg.Env = "k8s"
		env, err := resolvedEnv()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if env != "k8s" {
			t.Errorf("resolvedEnv = %q, want k8s", env)
		}
	})
}

func TestResolvedEnv_ExplicitDocker(t *testing.T) {
	withGlobalCfg(func() {
		globalCfg.Env = "docker"
		env, err := resolvedEnv()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if env != "docker" {
			t.Errorf("resolvedEnv = %q, want docker", env)
		}
	})
}

func TestResolvedEnv_Invalid(t *testing.T) {
	withGlobalCfg(func() {
		globalCfg.Env = "kubernetes"
		_, err := resolvedEnv()
		if err == nil {
			t.Error("expected error for invalid --env value, got nil")
		}
	})
}

func TestLoadClientConfig_GlobalServerOverride(t *testing.T) {
	withGlobalCfg(func() {
		globalCfg.Server = "10.0.0.1:36890"
		cfg := loadClientConfig()
		if cfg.Server != "10.0.0.1:36890" {
			t.Errorf("server = %q, want 10.0.0.1:36890", cfg.Server)
		}
	})
}
