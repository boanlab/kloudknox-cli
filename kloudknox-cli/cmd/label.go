// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/yaml"
)

func runLabel(args []string) error {
	fs := flag.NewFlagSet("label", flag.ContinueOnError)
	ns := fs.String("n", resolvedNamespace(), "namespace (pod only)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) < 3 {
		return errors.New("label: usage: kkctl label container|pod <name> <key=val> [key=val ...]")
	}
	kind := strings.ToLower(rest[0])
	name := rest[1]
	kvs := rest[2:]

	labels, err := parseKVs(kvs)
	if err != nil {
		return err
	}

	ctx := context.Background()
	switch kind {
	case "container":
		return labelContainer(ctx, name, labels)
	case "pod":
		return labelPod(ctx, name, *ns, labels)
	default:
		return fmt.Errorf("label: unsupported resource %q (supported: container, pod)", kind)
	}
}

// runInject reads selector labels from a KloudKnoxPolicy YAML and applies them
// to the target resource so the policy's selector will match it.
func runInject(args []string) error {
	fs := flag.NewFlagSet("inject", flag.ContinueOnError)
	file := fs.String("f", "", "path to a KloudKnoxPolicy YAML file (required)")
	target := fs.String("target", "", "resource type: container|pod (required)")
	name := fs.String("name", "", "resource name (required)")
	ns := fs.String("n", resolvedNamespace(), "namespace (pod only)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" && len(fs.Args()) > 0 {
		*name = fs.Args()[0]
	}
	if *file == "" {
		return errors.New("inject: -f is required")
	}
	if *target == "" {
		return errors.New("inject: --target is required (container|pod)")
	}
	if *name == "" {
		return errors.New("inject: target name is required")
	}

	data, err := readPolicyInput(*file)
	if err != nil {
		return err
	}
	docs, err := splitYAMLDocs(data)
	if err != nil {
		return err
	}

	ctx := context.Background()
	for _, doc := range docs {
		labels, err := extractSelectorLabels(doc)
		if err != nil {
			return err
		}
		if len(labels) == 0 {
			continue
		}
		switch strings.ToLower(*target) {
		case "container":
			if err := labelContainer(ctx, *name, labels); err != nil {
				return err
			}
		case "pod":
			if err := labelPod(ctx, *name, *ns, labels); err != nil {
				return err
			}
		default:
			return fmt.Errorf("inject: unsupported target %q (supported: container, pod)", *target)
		}
	}
	return nil
}

func labelContainer(ctx context.Context, name string, newLabels map[string]string) error {
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("label: docker client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	info, err := cli.ContainerInspect(ctx, name)
	if err != nil {
		return fmt.Errorf("label: inspect %s: %w", name, err)
	}

	existing := info.Config.Labels
	if existing == nil {
		existing = map[string]string{}
	}
	changed := false
	for k, v := range newLabels {
		if existing[k] != v {
			changed = true
			existing[k] = v
		}
	}
	if !changed {
		fmt.Printf("no label changes needed for container %s\n", name)
		return nil
	}

	// Docker doesn't allow changing labels in place, so recreate the container.
	fmt.Printf("recreating container %s to apply label changes...\n", name)

	reader, _ := cli.ImagePull(ctx, info.Config.Image, image.PullOptions{})
	if reader != nil {
		_, _ = io.Copy(io.Discard, reader)
		_ = reader.Close()
	}

	_ = cli.ContainerStop(ctx, info.ID, container.StopOptions{})
	_ = cli.ContainerRemove(ctx, info.ID, container.RemoveOptions{Force: true})

	newCfg := *info.Config
	newCfg.Labels = existing

	resp, err := cli.ContainerCreate(ctx, &newCfg, info.HostConfig, nil, nil, name)
	if err != nil {
		return fmt.Errorf("label: container create: %w", err)
	}
	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("label: container start: %w", err)
	}
	for k, v := range newLabels {
		fmt.Printf("labeled container/%s: %s=%s\n", name, k, v)
	}
	return nil
}

func labelPod(ctx context.Context, name, ns string, newLabels map[string]string) error {
	kc, err := buildKubeClients()
	if err != nil {
		return fmt.Errorf("label: %w", err)
	}

	labelsJSON := labelsToJSON(newLabels)
	patch := []byte(`{"metadata":{"labels":{` + labelsJSON + `}}}`)

	_, err = kc.Typed.CoreV1().Pods(ns).Patch(
		ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("label: patch pod %s/%s: %w", ns, name, err)
	}
	for k, v := range newLabels {
		fmt.Printf("labeled pod/%s: %s=%s\n", name, k, v)
	}
	return nil
}

func parseKVs(kvs []string) (map[string]string, error) {
	m := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid label format %q (expected key=value)", kv)
		}
		m[parts[0]] = parts[1]
	}
	return m, nil
}

func labelsToJSON(labels map[string]string) string {
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		parts = append(parts, fmt.Sprintf("%q:%q", k, v))
	}
	return strings.Join(parts, ",")
}

func extractSelectorLabels(raw []byte) (map[string]string, error) {
	var doc struct {
		Spec struct {
			Selector struct {
				MatchLabels map[string]string `yaml:"matchLabels"`
			} `yaml:"selector"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return doc.Spec.Selector.MatchLabels, nil
}
