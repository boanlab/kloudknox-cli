// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockerclient "github.com/docker/docker/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	outFlag := fs.String("o", "", "output format: table|wide|json (overrides global -o)")
	if args != nil {
		if err := fs.Parse(args); err != nil {
			return err
		}
	}
	out := resolvedOutput()
	if *outFlag != "" {
		out = *outFlag
	}

	env, err := resolvedEnv()
	if err != nil {
		return err
	}

	ctx := context.Background()
	switch env {
	case "k8s":
		return statusK8s(ctx, out)
	case "docker":
		return statusDocker(ctx, out)
	}
	return nil
}

func statusK8s(ctx context.Context, out string) error {
	kc, err := buildKubeClients()
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}
	ns := resolvedNamespace()

	ds, err := kc.Typed.AppsV1().DaemonSets(ns).Get(ctx, "kloudknox", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("status: daemonset not found — is KloudKnox installed? (%w)", err)
	}

	type nodeStatus struct {
		Name   string
		Status string
	}

	pods, _ := kc.Typed.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "boanlab.com/app=kloudknox",
	})

	var nodes []nodeStatus
	if pods != nil {
		for _, p := range pods.Items {
			s := "PENDING"
			for _, c := range p.Status.Conditions {
				if string(c.Type) == "Ready" && string(c.Status) == "True" {
					s = "OK"
				}
			}
			nodes = append(nodes, nodeStatus{
				Name:   p.Spec.NodeName,
				Status: s,
			})
		}
	}

	if out == "json" {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"mode":      "kubernetes",
			"namespace": ns,
			"desired":   ds.Status.DesiredNumberScheduled,
			"ready":     ds.Status.NumberReady,
			"nodes":     nodes,
		})
	}

	fmt.Println("KloudKnox Status")
	fmt.Printf("  Mode       : kubernetes\n")
	fmt.Printf("  Namespace  : %s\n", ns)
	fmt.Printf("  DaemonSet  : %d/%d ready\n\n",
		ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NODE\tSTATUS")
	for _, n := range nodes {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", n.Name, n.Status)
	}
	return tw.Flush()
}

func statusDocker(ctx context.Context, out string) error {
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("status: docker client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", kloudknoxContainer)),
	})
	if err != nil {
		return fmt.Errorf("status: list containers: %w", err)
	}

	containerState := "not found"
	if len(containers) > 0 {
		containerState = containers[0].State
	}

	policyCount := countFiles(kloudknoxPolicyDir)

	if out == "json" {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"mode":        "docker",
			"container":   containerState,
			"policyCount": policyCount,
		})
	}

	fmt.Println("KloudKnox Status")
	fmt.Printf("  Mode       : docker\n")
	fmt.Printf("  Container  : %s (%s)\n", kloudknoxContainer, containerState)
	fmt.Printf("  PolicyDir  : %s (%d files)\n", kloudknoxPolicyDir, policyCount)
	return nil
}

func runVersion(args []string) error {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	fmt.Printf("kkctl version: %s\n", Version)
	return nil
}

func countFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			n++
		}
	}
	return n
}
