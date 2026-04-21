// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"strings"
	"testing"
)

func TestParseKVs_Single(t *testing.T) {
	m, err := parseKVs([]string{"app=nginx"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["app"] != "nginx" {
		t.Errorf("m[app] = %q, want nginx", m["app"])
	}
}

func TestParseKVs_Multiple(t *testing.T) {
	m, err := parseKVs([]string{"app=nginx", "tier=web", "env=prod"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m) != 3 {
		t.Errorf("len(m) = %d, want 3", len(m))
	}
	if m["tier"] != "web" {
		t.Errorf("m[tier] = %q, want web", m["tier"])
	}
}

func TestParseKVs_ValueWithEquals(t *testing.T) {
	// value itself contains '='
	m, err := parseKVs([]string{"label=foo=bar"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["label"] != "foo=bar" {
		t.Errorf("m[label] = %q, want foo=bar", m["label"])
	}
}

func TestParseKVs_MissingEquals(t *testing.T) {
	_, err := parseKVs([]string{"app=nginx", "badformat"})
	if err == nil {
		t.Error("expected error for missing '=', got nil")
	}
	if !strings.Contains(err.Error(), "badformat") {
		t.Errorf("error should mention the bad token, got: %v", err)
	}
}

func TestParseKVs_Empty(t *testing.T) {
	m, err := parseKVs([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestLabelsToJSON_Single(t *testing.T) {
	got := labelsToJSON(map[string]string{"app": "nginx"})
	if !strings.Contains(got, `"app"`) || !strings.Contains(got, `"nginx"`) {
		t.Errorf("labelsToJSON = %q, expected app and nginx", got)
	}
}

func TestLabelsToJSON_Empty(t *testing.T) {
	got := labelsToJSON(map[string]string{})
	if got != "" {
		t.Errorf("labelsToJSON(empty) = %q, want empty", got)
	}
}

func TestExtractSelectorLabels_Valid(t *testing.T) {
	raw := []byte(`
spec:
  selector:
    matchLabels:
      app: nginx
      tier: web
`)
	labels, err := extractSelectorLabels(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if labels["app"] != "nginx" {
		t.Errorf("labels[app] = %q, want nginx", labels["app"])
	}
	if labels["tier"] != "web" {
		t.Errorf("labels[tier] = %q, want web", labels["tier"])
	}
}

func TestExtractSelectorLabels_NoMatchLabels(t *testing.T) {
	raw := []byte(`
spec:
  selector: {}
`)
	labels, err := extractSelectorLabels(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("expected empty labels, got %v", labels)
	}
}

func TestExtractSelectorLabels_NoSelector(t *testing.T) {
	raw := []byte(`spec: {}`)
	labels, err := extractSelectorLabels(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("expected empty labels, got %v", labels)
	}
}

func TestExtractSelectorLabels_InvalidYAML(t *testing.T) {
	_, err := extractSelectorLabels([]byte(`{not: valid:`))
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}
