// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/yaml"
)

func runDelete(args []string) error {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	file := fs.String("f", "", "path to a YAML file whose policies should be deleted ('-' for stdin)")
	policyDir := fs.String("policy-dir", kloudknoxPolicyDir, "policy directory (docker mode)")
	ignoreNotFound := fs.Bool("ignore-not-found", false, "exit 0 even if a policy does not exist")
	if err := fs.Parse(args); err != nil {
		return err
	}

	names, docs, err := collectDeleteTargets(*file, fs.Args())
	if err != nil {
		return err
	}

	env, err := resolvedEnv()
	if err != nil {
		return err
	}

	for i, name := range names {
		switch env {
		case "docker":
			path := filepath.Join(*policyDir, name+".yaml")
			switch err := os.Remove(path); {
			case err == nil:
				fmt.Printf("deleted policy/%s ← %s\n", name, path)
			case os.IsNotExist(err):
				if !*ignoreNotFound {
					return fmt.Errorf("delete %s: %s not found", name, path)
				}
			default:
				return fmt.Errorf("delete %s: %w", name, err)
			}
		case "k8s":
			raw := syntheticPolicyDoc(name)
			if i < len(docs) && docs[i] != nil {
				raw = docs[i]
			}
			if err := deleteAsK8sCRD(raw); err != nil {
				if *ignoreNotFound && apierrors.IsNotFound(err) {
					continue
				}
				return fmt.Errorf("delete %s: %w", name, err)
			}
			fmt.Printf("deleted policy/%s\n", name)
		}
	}
	return nil
}

// collectDeleteTargets resolves policy names from -f YAML (returning the
// parsed docs so k8s delete can use the exact GVK), or from positional args
// ("policy <name>...") where no doc bytes are needed.
func collectDeleteTargets(file string, positional []string) ([]string, [][]byte, error) {
	if file != "" {
		if len(positional) > 0 {
			return nil, nil, errors.New("delete: cannot combine -f with positional arguments")
		}
		data, err := readPolicyInput(file)
		if err != nil {
			return nil, nil, err
		}
		rawDocs, err := splitYAMLDocs(data)
		if err != nil {
			return nil, nil, err
		}
		if len(rawDocs) == 0 {
			return nil, nil, errors.New("delete: input contained no documents")
		}
		names := make([]string, 0, len(rawDocs))
		docs := make([][]byte, 0, len(rawDocs))
		for _, doc := range rawDocs {
			var m policyManifest
			if err := yaml.Unmarshal(doc, &m); err != nil {
				return nil, nil, fmt.Errorf("delete: failed to parse document: %w", err)
			}
			if m.Metadata.Name == "" {
				return nil, nil, errors.New("delete: metadata.name is required for each policy document")
			}
			names = append(names, m.Metadata.Name)
			docs = append(docs, doc)
		}
		return names, docs, nil
	}

	if len(positional) < 2 {
		return nil, nil, errors.New("delete: usage: kkctl delete policy <name> [<name>...]  OR  kkctl delete -f <file>")
	}
	if positional[0] != "policy" && positional[0] != "policies" {
		return nil, nil, fmt.Errorf("delete: unsupported resource %q (only 'policy' is supported)", positional[0])
	}
	return positional[1:], nil, nil
}

// syntheticPolicyDoc builds the minimum manifest deleteManifest needs to
// resolve the KloudKnoxPolicy GVK when the caller only has a name.
func syntheticPolicyDoc(name string) []byte {
	return fmt.Appendf(nil,
		"apiVersion: security.boanlab.com/v1\nkind: KloudKnoxPolicy\nmetadata:\n  name: %s\n",
		name,
	)
}

func deleteAsK8sCRD(raw []byte) error {
	kc, err := buildKubeClients()
	if err != nil {
		return err
	}
	ctx := context.Background()
	return deleteManifest(ctx, kc.Dynamic, kc.Mapper, raw)
}
