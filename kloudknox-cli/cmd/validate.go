// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"errors"
	"flag"
	"fmt"

	"sigs.k8s.io/yaml"
)

func runPolicy(args []string) error {
	if len(args) == 0 {
		return errors.New("policy: usage: kkctl policy validate -f <file>")
	}
	switch args[0] {
	case "validate":
		return runPolicyValidate(args[1:])
	default:
		return fmt.Errorf("policy: unknown subcommand %q (supported: validate)", args[0])
	}
}

func runPolicyValidate(args []string) error {
	fs := flag.NewFlagSet("policy validate", flag.ContinueOnError)
	file := fs.String("f", "", "path to a KloudKnoxPolicy YAML file (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("policy validate: -f is required")
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
		return errors.New("policy validate: no documents found")
	}

	allOK := true
	for _, doc := range docs {
		name, errs := validatePolicyDoc(doc)
		if name == "" {
			name = "<unknown>"
		}
		if len(errs) == 0 {
			fmt.Printf("[✓] %s: valid\n", name)
		} else {
			allOK = false
			for _, e := range errs {
				fmt.Printf("[✗] %s: %s\n", name, e)
			}
		}
	}
	if !allOK {
		return errors.New("one or more policies failed validation")
	}
	return nil
}

type policyDoc struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Action     string         `yaml:"action"`
		Selector   map[string]any `yaml:"selector"`
		ApplyToAll bool           `yaml:"applyToAll"`
	} `yaml:"spec"`
}

func validatePolicyDoc(raw []byte) (name string, errs []string) {
	var doc policyDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return "", []string{"YAML parse error: " + err.Error()}
	}

	name = doc.Metadata.Name

	if doc.APIVersion == "" {
		errs = append(errs, "missing apiVersion")
	}
	if doc.Kind != "KloudKnoxPolicy" {
		errs = append(errs, fmt.Sprintf("kind must be KloudKnoxPolicy, got %q", doc.Kind))
	}
	if doc.Metadata.Name == "" {
		errs = append(errs, "metadata.name is required")
	}
	if doc.Spec.Action != "" {
		switch doc.Spec.Action {
		case "Allow", "Audit", "Block":
		default:
			errs = append(errs, fmt.Sprintf("spec.action must be Allow|Audit|Block, got %q", doc.Spec.Action))
		}
	}
	if len(doc.Spec.Selector) == 0 && !doc.Spec.ApplyToAll {
		errs = append(errs, "spec.selector must not be empty (or set spec.applyToAll: true for docker/hybrid)")
	}

	return name, errs
}
