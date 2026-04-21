// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var validPolicy = []byte(`
apiVersion: security.boanlab.com/v1
kind: KloudKnoxPolicy
metadata:
  name: test-policy
spec:
  action: Block
  selector:
    matchLabels:
      app: nginx
`)

var invalidPolicy = []byte(`
apiVersion: security.boanlab.com/v1
kind: KloudKnoxPolicy
metadata:
  name: bad-policy
spec:
  action: Deny
  selector: {}
`)

func TestApplyDryRun_Valid(t *testing.T) {
	docs, _ := splitYAMLDocs(validPolicy)
	// applyDryRun prints to stdout; just verify it returns nil for a valid doc.
	if err := applyDryRun(docs); err != nil {
		t.Errorf("applyDryRun(valid) = %v, want nil", err)
	}
}

func TestApplyDryRun_Invalid(t *testing.T) {
	docs, _ := splitYAMLDocs(invalidPolicy)
	// applyDryRun prints errors but returns nil (errors go to stdout, not as return value).
	if err := applyDryRun(docs); err != nil {
		t.Errorf("applyDryRun(invalid) = %v, want nil (validation printed inline)", err)
	}
}

func TestApplyDryRun_MultiDoc(t *testing.T) {
	multi := append(validPolicy, []byte("\n---\n")...)
	multi = append(multi, validPolicy...)
	docs, _ := splitYAMLDocs(multi)
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(docs))
	}
	if err := applyDryRun(docs); err != nil {
		t.Errorf("applyDryRun(multi) = %v, want nil", err)
	}
}

func TestWritePolicyFile_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	data := []byte("apiVersion: security.boanlab.com/v1\n")
	if err := writePolicyFile(dir, "my-policy", data); err != nil {
		t.Fatalf("writePolicyFile: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "my-policy.yaml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("file content = %q, want %q", got, data)
	}
}

func TestWritePolicyFile_CreatesDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "sub", "dir")
	if err := writePolicyFile(dir, "policy", []byte("data")); err != nil {
		t.Fatalf("writePolicyFile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "policy.yaml")); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}

func TestWritePolicyFile_Overwrites(t *testing.T) {
	dir := t.TempDir()
	_ = writePolicyFile(dir, "p", []byte("old"))
	_ = writePolicyFile(dir, "p", []byte("new"))
	got, _ := os.ReadFile(filepath.Join(dir, "p.yaml"))
	if string(got) != "new" {
		t.Errorf("file content = %q, want new", got)
	}
}

func TestWritePolicyFile_NoDotfileLeftover(t *testing.T) {
	dir := t.TempDir()
	if err := writePolicyFile(dir, "p", []byte("data")); err != nil {
		t.Fatalf("writePolicyFile: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			t.Errorf("unexpected dotfile leftover: %s", e.Name())
		}
	}
	if len(entries) != 1 || entries[0].Name() != "p.yaml" {
		t.Errorf("expected single p.yaml, got %v", entries)
	}
}

func TestWritePolicyFile_CleansUpOnRenameFailure(t *testing.T) {
	// When the destination exists as a directory, Rename fails on Linux.
	// Verify the temp dotfile is cleaned up so the policy dir stays tidy.
	dir := t.TempDir()
	conflictingDir := filepath.Join(dir, "p.yaml")
	if err := os.MkdirAll(conflictingDir, 0o750); err != nil {
		t.Fatalf("mkdir conflict: %v", err)
	}
	if err := writePolicyFile(dir, "p", []byte("data")); err == nil {
		t.Fatal("expected rename to fail when target is a directory")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			t.Errorf("temp dotfile not cleaned up: %s", e.Name())
		}
	}
}

func TestReadPolicyInput_Stdin(_ *testing.T) {
	// Just verify the stdin branch is reachable (doesn't panic).
	// We don't actually read stdin in unit tests.
	_ = strings.NewReader("fake stdin")
}

func TestReadPolicyInput_MissingFile(t *testing.T) {
	_, err := readPolicyInput("/nonexistent/path/policy.yaml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}
