// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	dockerclient "github.com/docker/docker/client"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

func runDescribe(args []string) error {
	fs := flag.NewFlagSet("describe", flag.ContinueOnError)
	ns := fs.String("n", resolvedNamespace(), "namespace (k8s mode)")
	policyDir := fs.String("policy-dir", kloudknoxPolicyDir, "policy directory (docker mode)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) < 2 {
		return errors.New("describe: usage: kkctl describe policy|container|node <name>")
	}
	kind := rest[0]
	name := rest[1]

	switch kind {
	case "policy", "policies":
		return describePolicy(name, *ns, *policyDir)
	case "container":
		return describeContainer(name, *ns)
	case "node":
		return describeNode(name, *ns)
	default:
		return fmt.Errorf("describe: unsupported resource %q (supported: policy, container, node)", kind)
	}
}

func describePolicy(name, ns, policyDir string) error {
	env, err := resolvedEnv()
	if err != nil {
		return err
	}

	switch env {
	case "k8s":
		return describePolicyK8s(name, ns)
	case "docker":
		return describePolicyDocker(name, policyDir)
	}
	return fmt.Errorf("describe: unsupported env %q", env)
}

func describePolicyK8s(name, ns string) error {
	kc, err := buildKubeClients()
	if err != nil {
		return err
	}
	gk := schema.GroupKind{Group: "security.boanlab.com", Kind: "KloudKnoxPolicy"}
	mapping, err := kc.Mapper.RESTMapping(gk)
	if err != nil {
		return err
	}
	obj, err := kc.Dynamic.Resource(mapping.Resource).Namespace(ns).Get(
		context.Background(), name, metav1.GetOptions{},
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("describe: policy %q not found in namespace %q", name, ns)
		}
		return err
	}
	b, err := yaml.Marshal(obj.Object)
	if err != nil {
		return err
	}
	fmt.Println(string(bytes.TrimRight(b, "\n")))
	return nil
}

func describePolicyDocker(name, dir string) error {
	path := filepath.Join(dir, name+".yaml")
	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("describe: policy %q not found at %s", name, path)
		}
		return fmt.Errorf("describe: read %s: %w", path, err)
	}
	fmt.Println(string(bytes.TrimRight(data, "\n")))
	return nil
}

func describeContainer(name, ns string) error {
	env, err := resolvedEnv()
	if err != nil {
		return err
	}

	switch env {
	case "k8s":
		return describeContainerK8s(name, ns)
	case "docker":
		return describeContainerDocker(name)
	}
	return fmt.Errorf("describe: unsupported env %q", env)
}

// describeContainerK8s resolves a container by name across the namespace's pods.
// The namespace defaults to the global --namespace (kloudknox); pass
// --namespace to inspect workloads elsewhere.
func describeContainerK8s(name, ns string) error {
	kc, err := buildKubeClients()
	if err != nil {
		return fmt.Errorf("describe container: %w", err)
	}

	pods, err := kc.Typed.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("describe container %s: %w", name, err)
	}

	for i := range pods.Items {
		pod := &pods.Items[i]
		for _, c := range pod.Spec.Containers {
			if c.Name != name {
				continue
			}
			fmt.Printf("Name:      %s\n", c.Name)
			fmt.Printf("Pod:       %s/%s\n", pod.Namespace, pod.Name)
			fmt.Printf("Node:      %s\n", pod.Spec.NodeName)
			fmt.Printf("Image:     %s\n", c.Image)
			fmt.Printf("Status:    %s\n", pod.Status.Phase)
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.Name == name {
					fmt.Printf("Ready:     %t (restarts: %d)\n", cs.Ready, cs.RestartCount)
					fmt.Printf("ImageID:   %s\n", cs.ImageID)
					break
				}
			}
			if len(pod.Labels) > 0 {
				fmt.Println("Pod labels:")
				for k, v := range pod.Labels {
					fmt.Printf("  %s=%s\n", k, v)
				}
			}
			return nil
		}
	}
	return fmt.Errorf("describe: container %q not found in namespace %q", name, ns)
}

func describeContainerDocker(name string) error {
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("describe: docker client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	info, err := cli.ContainerInspect(context.Background(), name)
	if err != nil {
		return fmt.Errorf("describe: container %q not found: %w", name, err)
	}

	fmt.Printf("Name:    %s\n", info.Name)
	fmt.Printf("ID:      %s\n", info.ID[:12])
	fmt.Printf("Image:   %s\n", info.Config.Image)
	fmt.Printf("Status:  %s\n", info.State.Status)
	fmt.Printf("Started: %s\n", info.State.StartedAt)
	if len(info.Config.Labels) > 0 {
		fmt.Println("Labels:")
		for k, v := range info.Config.Labels {
			fmt.Printf("  %s=%s\n", k, v)
		}
	}
	return nil
}

func describeNode(nodeName, ns string) error {
	env, err := resolvedEnv()
	if err != nil {
		return err
	}
	if env != "k8s" {
		return errors.New("describe node: only supported in k8s mode")
	}

	kc, err := buildKubeClients()
	if err != nil {
		return fmt.Errorf("describe node: %w", err)
	}
	ctx := context.Background()

	node, err := kc.Typed.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("describe node %s: %w", nodeName, err)
	}

	fmt.Printf("Node:       %s\n", node.Name)
	fmt.Printf("Arch:       %s\n", node.Status.NodeInfo.Architecture)
	fmt.Printf("OS:         %s\n", node.Status.NodeInfo.OSImage)
	fmt.Printf("Kernel:     %s\n", node.Status.NodeInfo.KernelVersion)
	fmt.Printf("Runtime:    %s\n", node.Status.NodeInfo.ContainerRuntimeVersion)

	pods, err := kc.Typed.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "boanlab.com/app=kloudknox",
		FieldSelector: "spec.nodeName=" + nodeName,
	})
	if err == nil && len(pods.Items) > 0 {
		p := pods.Items[0]
		fmt.Printf("KloudKnox:  pod/%s (%s)\n", p.Name, p.Status.Phase)
	}

	events, err := kc.Typed.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
		FieldSelector: "involvedObject.kind=Node,involvedObject.name=" + nodeName,
	})
	if err == nil && len(events.Items) > 0 {
		fmt.Println("\nEvents:")
		for _, e := range events.Items {
			fmt.Printf("  %s  %s  %s\n", e.LastTimestamp.Format("15:04:05"), e.Type, e.Message)
		}
	}
	return nil
}
