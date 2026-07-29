// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

func runGet(args []string) error {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	outFlag := fs.String("o", "", "output format: table|wide|json|yaml (overrides global -o)")
	policyDir := fs.String("policy-dir", kloudknoxPolicyDir, "policy directory (docker mode)")
	allNS := fs.Bool("A", false, "list across all namespaces (k8s mode)")

	// Hoist the resource kind so users can write `kkctl get policies -A -o wide`.
	// Go's flag package stops parsing at the first positional, which would
	// otherwise leave `-A` and `-o wide` unparsed.
	var kind string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		kind = strings.ToLower(args[0])
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if kind == "" {
		rest := fs.Args()
		if len(rest) == 0 {
			return errors.New("get: usage: kkctl get policies|nodes")
		}
		kind = strings.ToLower(rest[0])
	}

	out := resolvedOutput()
	if *outFlag != "" {
		out = *outFlag
	}

	switch kind {
	case "policy", "policies":
		return getPolicies(out, *policyDir, *allNS)
	case "node", "nodes":
		return getNodes(out)
	default:
		return fmt.Errorf("get: unsupported resource %q", kind)
	}
}

type policySummary struct {
	NamespaceName string     `json:"NamespaceName"`
	PolicyName    string     `json:"PolicyName"`
	Action        string     `json:"Action"`
	Status        string     `json:"Status"`
	Process       []any      `json:"Process"`
	File          []any      `json:"File"`
	Network       []any      `json:"Network"`
	Capability    []any      `json:"Capability"`
	IPC           ipcSummary `json:"IPC"`
}

// ipcSummary mirrors spec.ipc, which groups its rules by sub-domain rather than
// holding a flat list like the other rule kinds.
type ipcSummary struct {
	Unix   []any `json:"Unix"`
	Signal []any `json:"Signal"`
	Ptrace []any `json:"Ptrace"`
}

// count returns the number of IPC rules across every sub-domain.
func (i ipcSummary) count() int {
	return len(i.Unix) + len(i.Signal) + len(i.Ptrace)
}

func getPolicies(out, policyDir string, allNS bool) error {
	env, err := resolvedEnv()
	if err != nil {
		return err
	}

	var policies []policySummary
	switch env {
	case "k8s":
		policies, err = listPoliciesK8s(allNS)
	case "docker":
		policies, err = listPoliciesDocker(policyDir)
	}
	if err != nil {
		return err
	}

	return renderPolicies(policies, out)
}

func listPoliciesK8s(allNS bool) ([]policySummary, error) {
	kc, err := buildKubeClients()
	if err != nil {
		return nil, err
	}
	gk := schema.GroupKind{Group: "security.boanlab.com", Kind: "KloudKnoxPolicy"}
	mapping, err := kc.Mapper.RESTMapping(gk)
	if err != nil {
		return nil, fmt.Errorf("get policies: %w", err)
	}

	nri := kc.Dynamic.Resource(mapping.Resource)
	ctx := context.Background()
	var list *unstructured.UnstructuredList
	if allNS {
		list, err = nri.List(ctx, metav1.ListOptions{})
	} else {
		list, err = nri.Namespace(resolvedNamespace()).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, err
	}

	out := make([]policySummary, 0, len(list.Items))
	for _, item := range list.Items {
		out = append(out, summarizeUnstructured(item.Object))
	}
	return out, nil
}

func listPoliciesDocker(dir string) ([]policySummary, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get policies: %w", err)
	}
	var out []policySummary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name())) // #nosec G304
		if err != nil {
			return nil, fmt.Errorf("get policies: read %s: %w", e.Name(), err)
		}
		var raw map[string]any
		if err := yaml.Unmarshal(data, &raw); err != nil {
			continue
		}
		out = append(out, summarizeUnstructured(raw))
	}
	return out, nil
}

func summarizeUnstructured(obj map[string]any) policySummary {
	s := policySummary{}
	if md, ok := obj["metadata"].(map[string]any); ok {
		if v, ok := md["namespace"].(string); ok {
			s.NamespaceName = v
		}
		if v, ok := md["name"].(string); ok {
			s.PolicyName = v
		}
	}
	if spec, ok := obj["spec"].(map[string]any); ok {
		if v, ok := spec["action"].(string); ok {
			s.Action = v
		}
		if v, ok := spec["process"].([]any); ok {
			s.Process = v
		}
		if v, ok := spec["file"].([]any); ok {
			s.File = v
		}
		if v, ok := spec["network"].([]any); ok {
			s.Network = v
		}
		if v, ok := spec["capability"].([]any); ok {
			s.Capability = v
		}
		if ipc, ok := spec["ipc"].(map[string]any); ok {
			if v, ok := ipc["unix"].([]any); ok {
				s.IPC.Unix = v
			}
			if v, ok := ipc["signal"].([]any); ok {
				s.IPC.Signal = v
			}
			if v, ok := ipc["ptrace"].([]any); ok {
				s.IPC.Ptrace = v
			}
		}
	}
	if st, ok := obj["status"].(map[string]any); ok {
		if v, ok := st["status"].(string); ok {
			s.Status = v
		}
	}
	return s
}

func renderPolicies(policies []policySummary, out string) error {
	switch out {
	case "json":
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"policies": policies})
	case "yaml":
		b, err := yaml.Marshal(map[string]any{"policies": policies})
		if err != nil {
			return err
		}
		_, _ = os.Stdout.Write(b)
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if out == "wide" {
		_, _ = fmt.Fprintln(tw, "NAMESPACE\tNAME\tACTION\tSTATUS\tPROCESS\tFILE\tNETWORK\tCAPABILITY\tIPC")
	} else {
		_, _ = fmt.Fprintln(tw, "NAMESPACE\tNAME\tACTION\tSTATUS")
	}
	for _, p := range policies {
		if out == "wide" {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%d\t%d\t%d\t%d\n",
				emptyAsDash(p.NamespaceName), p.PolicyName, emptyAsDash(p.Action),
				emptyAsDash(p.Status), len(p.Process), len(p.File), len(p.Network),
				len(p.Capability), p.IPC.count())
		} else {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				emptyAsDash(p.NamespaceName), p.PolicyName, emptyAsDash(p.Action),
				emptyAsDash(p.Status))
		}
	}
	return tw.Flush()
}

func getNodes(out string) error {
	env, err := resolvedEnv()
	if err != nil {
		return err
	}
	if env != "k8s" {
		return errors.New("get nodes: only supported in k8s mode")
	}

	kc, err := buildKubeClients()
	if err != nil {
		return fmt.Errorf("get nodes: %w", err)
	}

	pods, err := kc.Typed.CoreV1().Pods(resolvedNamespace()).List(context.Background(), metav1.ListOptions{
		LabelSelector: "boanlab.com/app=kloudknox",
	})
	if err != nil {
		return fmt.Errorf("get nodes: %w", err)
	}

	if out == "json" {
		type nodeSummary struct {
			Node   string `json:"node"`
			Status string `json:"status"`
		}
		var nodes []nodeSummary
		for _, p := range pods.Items {
			s := "PENDING"
			for _, c := range p.Status.Conditions {
				if string(c.Type) == "Ready" && string(c.Status) == "True" {
					s = "OK"
				}
			}
			nodes = append(nodes, nodeSummary{Node: p.Spec.NodeName, Status: s})
		}
		return json.NewEncoder(os.Stdout).Encode(nodes)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NODE\tSTATUS")
	for _, p := range pods.Items {
		s := "PENDING"
		for _, c := range p.Status.Conditions {
			if string(c.Type) == "Ready" && string(c.Status) == "True" {
				s = "OK"
			}
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", p.Spec.NodeName, s)
	}
	return tw.Flush()
}

func emptyAsDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
