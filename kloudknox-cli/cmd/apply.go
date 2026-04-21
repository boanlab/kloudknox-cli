// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

// policyManifest is the minimal view of a KloudKnoxPolicy document needed
// to route it to the right backend (CRD object in k8s, YAML file in docker).
type policyManifest struct {
	APIVersion string         `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`
	Kind       string         `json:"kind,omitempty"       yaml:"kind,omitempty"`
	Metadata   policyMetadata `json:"metadata,omitzero"    yaml:"metadata,omitempty"`
	Spec       map[string]any `json:"spec,omitempty"       yaml:"spec,omitempty"`
}

type policyMetadata struct {
	Name      string            `json:"name,omitempty"      yaml:"name,omitempty"`
	Namespace string            `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"    yaml:"labels,omitempty"`
}

func runApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	file := fs.String("f", "", "path to a YAML file (required; use '-' for stdin)")
	policyDir := fs.String("policy-dir", kloudknoxPolicyDir, "policy directory (docker mode)")
	dryRun := fs.Bool("dry-run", false, "validate and print what would be applied without sending")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("apply: -f is required")
	}

	data, err := readPolicyInput(*file)
	if err != nil {
		return err
	}
	docs, err := splitYAMLDocs(data)
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return errors.New("apply: input contained no documents")
	}

	if *dryRun {
		return applyDryRun(docs)
	}

	env, err := resolvedEnv()
	if err != nil {
		return err
	}

	for _, doc := range docs {
		var m policyManifest
		if err := yaml.Unmarshal(doc, &m); err != nil {
			return fmt.Errorf("apply: failed to parse document: %w", err)
		}
		if m.Metadata.Name == "" {
			return errors.New("apply: metadata.name is required for each policy document")
		}

		switch env {
		case "docker":
			if err := writePolicyFile(*policyDir, m.Metadata.Name, doc); err != nil {
				return fmt.Errorf("apply %s: %w", m.Metadata.Name, err)
			}
			fmt.Printf("applied policy/%s → %s/%s.yaml\n", m.Metadata.Name, *policyDir, m.Metadata.Name)
		case "k8s":
			if err := applyAsK8sCRD(doc); err != nil {
				return fmt.Errorf("apply %s: %w", m.Metadata.Name, err)
			}
			fmt.Printf("applied policy/%s\n", m.Metadata.Name)
		}
	}
	return nil
}

func applyDryRun(docs [][]byte) error {
	for _, doc := range docs {
		var m policyManifest
		if err := yaml.Unmarshal(doc, &m); err != nil {
			return fmt.Errorf("apply --dry-run: parse: %w", err)
		}
		_, errs := validatePolicyDoc(doc)
		if len(errs) > 0 {
			for _, e := range errs {
				fmt.Printf("[✗] %s: %s\n", m.Metadata.Name, e)
			}
		} else {
			fmt.Printf("[dry-run] would apply policy/%s\n", m.Metadata.Name)
		}
	}
	return nil
}

// writePolicyFile writes a policy YAML atomically using write-then-rename.
// The temp file is created in the same directory (so Rename stays on one FS)
// and starts with "." so the PolicyFileLoader's dotfile filter skips any
// intermediate fsnotify events for it. The rename to the final ".yaml" name
// produces a single CREATE/RENAME event that the loader observes atomically.
func writePolicyFile(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	final := filepath.Join(dir, name+".yaml")

	tmp, err := os.CreateTemp(dir, "."+name+".yaml.*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, final); err != nil {
		cleanup()
		return err
	}
	return nil
}

func applyAsK8sCRD(raw []byte) error {
	kc, err := buildKubeClients()
	if err != nil {
		return err
	}
	ctx := context.Background()
	return applyManifest(ctx, kc.Dynamic, kc.Mapper, raw)
}

func readPolicyInput(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

// splitYAMLDocs splits a YAML stream on `^---` lines, dropping empty docs.
func splitYAMLDocs(data []byte) ([][]byte, error) {
	var (
		docs    [][]byte
		current bytes.Buffer
	)
	flush := func() {
		if len(bytes.TrimSpace(current.Bytes())) == 0 {
			return
		}
		out := make([]byte, current.Len())
		copy(out, current.Bytes())
		docs = append(docs, out)
	}
	for line := range bytes.SplitSeq(data, []byte("\n")) {
		trimmed := bytes.TrimRight(line, "\r")
		if bytes.Equal(bytes.TrimSpace(trimmed), []byte("---")) {
			flush()
			current.Reset()
			continue
		}
		current.Write(line)
		current.WriteByte('\n')
	}
	flush()
	return docs, nil
}
