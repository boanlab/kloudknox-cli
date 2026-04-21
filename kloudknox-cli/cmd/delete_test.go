// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func writeTempPolicy(t *testing.T, content []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestCollectDeleteTargets_FileSingleDoc(t *testing.T) {
	path := writeTempPolicy(t, validPolicy)
	names, docs, err := collectDeleteTargets(path, nil)
	if err != nil {
		t.Fatalf("collectDeleteTargets: %v", err)
	}
	if !reflect.DeepEqual(names, []string{"test-policy"}) {
		t.Errorf("names = %v, want [test-policy]", names)
	}
	if len(docs) != 1 {
		t.Errorf("len(docs) = %d, want 1", len(docs))
	}
}

func TestCollectDeleteTargets_FileMultiDoc(t *testing.T) {
	second := []byte(`
apiVersion: security.boanlab.com/v1
kind: KloudKnoxPolicy
metadata:
  name: other-policy
spec:
  action: Audit
`)
	multi := append([]byte{}, validPolicy...)
	multi = append(multi, []byte("\n---\n")...)
	multi = append(multi, second...)

	path := writeTempPolicy(t, multi)
	names, docs, err := collectDeleteTargets(path, nil)
	if err != nil {
		t.Fatalf("collectDeleteTargets: %v", err)
	}
	if !reflect.DeepEqual(names, []string{"test-policy", "other-policy"}) {
		t.Errorf("names = %v, want [test-policy other-policy]", names)
	}
	if len(docs) != 2 {
		t.Errorf("len(docs) = %d, want 2", len(docs))
	}
}

func TestCollectDeleteTargets_FileMissingName(t *testing.T) {
	noName := []byte(`
apiVersion: security.boanlab.com/v1
kind: KloudKnoxPolicy
metadata: {}
spec:
  action: Block
`)
	path := writeTempPolicy(t, noName)
	_, _, err := collectDeleteTargets(path, nil)
	if err == nil {
		t.Fatal("expected error for missing metadata.name, got nil")
	}
	if !strings.Contains(err.Error(), "metadata.name is required") {
		t.Errorf("error = %v, want contains 'metadata.name is required'", err)
	}
}

func TestCollectDeleteTargets_FileEmpty(t *testing.T) {
	path := writeTempPolicy(t, []byte("   \n---\n\n"))
	_, _, err := collectDeleteTargets(path, nil)
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
	if !strings.Contains(err.Error(), "no documents") {
		t.Errorf("error = %v, want contains 'no documents'", err)
	}
}

func TestCollectDeleteTargets_FileAndPositionalConflict(t *testing.T) {
	path := writeTempPolicy(t, validPolicy)
	_, _, err := collectDeleteTargets(path, []string{"policy", "foo"})
	if err == nil {
		t.Fatal("expected error when -f and positional args combined, got nil")
	}
	if !strings.Contains(err.Error(), "cannot combine") {
		t.Errorf("error = %v, want contains 'cannot combine'", err)
	}
}

func TestCollectDeleteTargets_PositionalSingle(t *testing.T) {
	names, docs, err := collectDeleteTargets("", []string{"policy", "my-policy"})
	if err != nil {
		t.Fatalf("collectDeleteTargets: %v", err)
	}
	if !reflect.DeepEqual(names, []string{"my-policy"}) {
		t.Errorf("names = %v, want [my-policy]", names)
	}
	if docs != nil {
		t.Errorf("docs = %v, want nil for positional form", docs)
	}
}

func TestCollectDeleteTargets_PositionalMultiple(t *testing.T) {
	names, _, err := collectDeleteTargets("", []string{"policies", "a", "b", "c"})
	if err != nil {
		t.Fatalf("collectDeleteTargets: %v", err)
	}
	if !reflect.DeepEqual(names, []string{"a", "b", "c"}) {
		t.Errorf("names = %v, want [a b c]", names)
	}
}

func TestCollectDeleteTargets_PositionalUnsupportedResource(t *testing.T) {
	_, _, err := collectDeleteTargets("", []string{"pod", "foo"})
	if err == nil {
		t.Fatal("expected error for unsupported resource, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported resource") {
		t.Errorf("error = %v, want contains 'unsupported resource'", err)
	}
}

func TestCollectDeleteTargets_PositionalTooFewArgs(t *testing.T) {
	cases := [][]string{nil, {"policy"}}
	for _, args := range cases {
		if _, _, err := collectDeleteTargets("", args); err == nil {
			t.Errorf("args=%v: expected error, got nil", args)
		}
	}
}

func TestSyntheticPolicyDoc_ParsesWithExpectedName(t *testing.T) {
	raw := syntheticPolicyDoc("my-policy")
	var m policyManifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if m.APIVersion != "security.boanlab.com/v1" {
		t.Errorf("apiVersion = %q, want security.boanlab.com/v1", m.APIVersion)
	}
	if m.Kind != "KloudKnoxPolicy" {
		t.Errorf("kind = %q, want KloudKnoxPolicy", m.Kind)
	}
	if m.Metadata.Name != "my-policy" {
		t.Errorf("name = %q, want my-policy", m.Metadata.Name)
	}
}
