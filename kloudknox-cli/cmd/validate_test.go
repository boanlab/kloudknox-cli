// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"strings"
	"testing"
)

var validPolicyYAML = []byte(`
apiVersion: security.boanlab.com/v1
kind: KloudKnoxPolicy
metadata:
  name: nginx-hardening
spec:
  action: Block
  selector:
    matchLabels:
      app: nginx
`)

func TestValidatePolicyDoc_Valid(t *testing.T) {
	name, errs := validatePolicyDoc(validPolicyYAML)
	if name != "nginx-hardening" {
		t.Errorf("name = %q, want nginx-hardening", name)
	}
	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestValidatePolicyDoc_MissingName(t *testing.T) {
	raw := []byte(`
apiVersion: security.boanlab.com/v1
kind: KloudKnoxPolicy
metadata: {}
spec:
  action: Allow
  selector:
    matchLabels:
      app: nginx
`)
	_, errs := validatePolicyDoc(raw)
	if !containsAny(errs, "metadata.name") {
		t.Errorf("expected metadata.name error, got %v", errs)
	}
}

func TestValidatePolicyDoc_BadAction(t *testing.T) {
	raw := []byte(`
apiVersion: security.boanlab.com/v1
kind: KloudKnoxPolicy
metadata:
  name: bad
spec:
  action: Deny
  selector:
    matchLabels:
      app: nginx
`)
	_, errs := validatePolicyDoc(raw)
	if !containsAny(errs, "Allow|Audit|Block") {
		t.Errorf("expected action error, got %v", errs)
	}
}

func TestValidatePolicyDoc_EmptySelector(t *testing.T) {
	raw := []byte(`
apiVersion: security.boanlab.com/v1
kind: KloudKnoxPolicy
metadata:
  name: no-selector
spec:
  action: Block
  selector: {}
`)
	_, errs := validatePolicyDoc(raw)
	if !containsAny(errs, "selector") {
		t.Errorf("expected selector error, got %v", errs)
	}
}

func TestValidatePolicyDoc_MissingAPIVersion(t *testing.T) {
	raw := []byte(`
kind: KloudKnoxPolicy
metadata:
  name: no-api
spec:
  action: Allow
  selector:
    matchLabels:
      app: test
`)
	_, errs := validatePolicyDoc(raw)
	if !containsAny(errs, "apiVersion") {
		t.Errorf("expected apiVersion error, got %v", errs)
	}
}

func TestValidatePolicyDoc_WrongKind(t *testing.T) {
	raw := []byte(`
apiVersion: security.boanlab.com/v1
kind: NetworkPolicy
metadata:
  name: wrong-kind
spec:
  action: Allow
  selector:
    matchLabels:
      app: test
`)
	_, errs := validatePolicyDoc(raw)
	if !containsAny(errs, "KloudKnoxPolicy") {
		t.Errorf("expected kind error, got %v", errs)
	}
}

func TestValidatePolicyDoc_MultipleErrors(t *testing.T) {
	raw := []byte(`
kind: Wrong
metadata: {}
spec:
  action: Deny
  selector: {}
`)
	_, errs := validatePolicyDoc(raw)
	if len(errs) < 3 {
		t.Errorf("expected at least 3 errors (apiVersion, kind, name, action, selector), got %d: %v", len(errs), errs)
	}
}

func TestValidatePolicyDoc_NoAction(t *testing.T) {
	// action is optional — empty action should not error
	raw := []byte(`
apiVersion: security.boanlab.com/v1
kind: KloudKnoxPolicy
metadata:
  name: no-action
spec:
  selector:
    matchLabels:
      app: test
`)
	_, errs := validatePolicyDoc(raw)
	if containsAny(errs, "action") {
		t.Errorf("empty action should not produce error, got %v", errs)
	}
}

func TestValidatePolicyDoc_ParseError(t *testing.T) {
	raw := []byte(`{not: valid: yaml:`)
	_, errs := validatePolicyDoc(raw)
	if len(errs) == 0 {
		t.Error("expected parse error for malformed YAML")
	}
}

func TestValidatePolicyDoc_ApplyToAllAllowsEmptySelector(t *testing.T) {
	raw := []byte(`
apiVersion: security.boanlab.com/v1
kind: KloudKnoxPolicy
metadata:
  name: host-wide
spec:
  applyToAll: true
  action: Block
`)
	_, errs := validatePolicyDoc(raw)
	if containsAny(errs, "selector") {
		t.Errorf("applyToAll should permit empty selector, got %v", errs)
	}
}

// containsAny returns true if any error string contains substr.
func containsAny(errs []string, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}
