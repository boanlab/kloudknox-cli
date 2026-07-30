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
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/strslice"
	dockerclient "github.com/docker/docker/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"
)

const (
	defaultImage         = "boanlab/kloudknox:latest"
	kloudknoxContainer   = "kloudknox"
	kloudknoxPolicyDir   = "/etc/kloudknox/policies"
	kloudknoxComposeFile = "/etc/kloudknox/docker-compose.yaml"
)

func runInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	img := fs.String("image", "", "container image to deploy (default: "+defaultImage+")")
	wait := fs.Bool("wait", true, "wait for rollout to complete (K8s only)")
	skipWebhook := fs.Bool("skip-apparmor-webhook", false, "force the AppArmor mutation webhook out (default: installed only when an agent uses AppArmor)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *img == "" {
		*img = defaultImage
	}

	env, err := resolvedEnv()
	if err != nil {
		return err
	}

	ctx := context.Background()
	switch env {
	case "k8s":
		return installK8s(ctx, *img, *wait, *skipWebhook)
	case "docker":
		return installDocker(ctx, *img)
	}
	return nil
}

func installK8s(ctx context.Context, img string, wait bool, skipWebhook bool) error {
	kc, err := buildKubeClients()
	if err != nil {
		return fmt.Errorf("install: %w", err)
	}
	ns := resolvedNamespace()

	applyRaw := func(raw []byte) error {
		if img != defaultImage {
			raw = bytes.ReplaceAll(raw, []byte(defaultImage), []byte(img))
		}
		docs, err := splitYAMLDocs(raw)
		if err != nil {
			return err
		}
		for _, doc := range docs {
			if err := applyManifest(ctx, kc.Dynamic, kc.Mapper, doc); err != nil {
				return fmt.Errorf("install: apply manifest: %w", err)
			}
		}
		return nil
	}

	for _, raw := range k8sManifestsWithoutWebhook() {
		if err := applyRaw(raw); err != nil {
			return err
		}
	}
	fmt.Println("KloudKnox manifests applied.")

	if wait {
		fmt.Print("Waiting for DaemonSet rollout...")
		if err := waitDaemonSet(ctx, kc, ns); err != nil {
			return fmt.Errorf("install: %w", err)
		}
		fmt.Println(" done.")
	}

	if skipWebhook {
		fmt.Println("AppArmor webhook: skipped (--skip-apparmor-webhook).")
	} else {
		install, reason := apparmorWebhookNeeded(ctx, kc, ns, wait)
		if install {
			if err := applyRaw(manifestApparmorWebhook()); err != nil {
				return err
			}
			fmt.Printf("AppArmor webhook: installed — %s\n", reason)
		} else {
			fmt.Printf("AppArmor webhook: skipped — %s\n", reason)
		}
	}

	return runStatus(nil)
}

// apparmorWebhookNeeded reports whether the cluster needs the AppArmor webhook.
//
// The webhook annotates pods with a `localhost/kloudknox-*` profile, and agents
// only generate those under the AppArmor enforcer — on a BPF-LSM node the
// annotation can never be satisfied and the pod fails to start. Each agent
// reports its enforcer at startup, so ask them. Anything short of every agent
// reporting BPF-LSM installs the webhook, so an undetected AppArmor node is
// never left without injection.
func apparmorWebhookNeeded(ctx context.Context, kc *kubeClients, ns string, waited bool) (bool, string) {
	if !waited {
		return true, "enforcer unknown without --wait"
	}

	pods, err := kc.Typed.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "boanlab.com/app=kloudknox",
	})
	if err != nil || len(pods.Items) == 0 {
		return true, "could not list agent pods"
	}
	total := len(pods.Items)

	// The DaemonSet has no readiness probe, so a pod reports Ready as soon as its
	// container starts — seconds before the enforcer line reaches the log. Poll
	// until every agent has reported.
	apparmor, bpf := 0, 0
	deadline := time.Now().Add(90 * time.Second)
	for {
		apparmor, bpf = 0, 0
		for i := range pods.Items {
			switch agentEnforcer(ctx, kc, ns, pods.Items[i].Name) {
			case "apparmor":
				apparmor++
			case "bpf":
				bpf++
			}
		}
		if apparmor+bpf >= total || time.Now().After(deadline) {
			break
		}
		time.Sleep(3 * time.Second)
	}

	switch {
	case apparmor+bpf < total:
		return true, fmt.Sprintf("only %d of %d agents reported an enforcer", apparmor+bpf, total)
	case apparmor == 0:
		return false, fmt.Sprintf("all %d agents use BPF-LSM", total)
	default:
		return true, fmt.Sprintf("%d of %d agents use AppArmor", apparmor, total)
	}
}

// agentEnforcer returns "apparmor", "bpf", or "" from an agent's startup log.
func agentEnforcer(ctx context.Context, kc *kubeClients, ns, pod string) string {
	stream, err := kc.Typed.CoreV1().Pods(ns).
		GetLogs(pod, &corev1.PodLogOptions{}).Stream(ctx)
	if err != nil {
		return ""
	}
	defer func() { _ = stream.Close() }()

	data, err := io.ReadAll(stream)
	if err != nil {
		return ""
	}
	switch {
	case bytes.Contains(data, []byte("Started AppArmor Enforcer")):
		return "apparmor"
	case bytes.Contains(data, []byte("Started BPF-LSM Enforcer")):
		return "bpf"
	}
	return ""
}

func installDocker(ctx context.Context, img string) error {
	if err := os.MkdirAll(kloudknoxPolicyDir, 0o750); err != nil {
		return fmt.Errorf("install: mkdir %s: %w", kloudknoxPolicyDir, err)
	}

	composeData := bytes.ReplaceAll(dockerComposeYAML,
		[]byte(defaultImage), []byte(img))
	if err := os.WriteFile(kloudknoxComposeFile, composeData, 0o600); err != nil {
		return fmt.Errorf("install: write compose file: %w", err)
	}

	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("install: docker client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	fmt.Printf("Pulling image %s...\n", img)
	reader, err := cli.ImagePull(ctx, img, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("install: image pull: %w", err)
	}
	_, _ = io.Copy(io.Discard, reader)
	_ = reader.Close()

	existing, _ := cli.ContainerInspect(ctx, kloudknoxContainer)
	if existing.ID != "" {
		_ = cli.ContainerStop(ctx, existing.ID, container.StopOptions{})
		_ = cli.ContainerRemove(ctx, existing.ID, container.RemoveOptions{Force: true})
	}

	resp, err := cli.ContainerCreate(ctx,
		&container.Config{
			Image: img,
			Cmd: []string{
				"-mode=docker",
				"-dockerEndpoint=unix:///var/run/docker.sock",
				"-policyDir=" + kloudknoxPolicyDir,
				"-defaultNamespace=docker",
				"-logPath=stdout",
			},
		},
		&container.HostConfig{
			Binds: []string{
				"/var/run/docker.sock:/var/run/docker.sock:ro",
				"/sys/kernel/security:/sys/kernel/security",
				"/sys/fs/bpf:/sys/fs/bpf:rshared",
				"/sys/fs/cgroup:/sys/fs/cgroup:ro",
				"/etc/apparmor.d:/etc/apparmor.d",
				kloudknoxPolicyDir + ":" + kloudknoxPolicyDir + ":ro",
			},
			CapAdd:        strslice.StrSlice{"SYS_ADMIN", "BPF", "PERFMON", "NET_ADMIN", "MAC_ADMIN"},
			SecurityOpt:   []string{"apparmor=unconfined"},
			NetworkMode:   container.NetworkMode("host"),
			PidMode:       container.PidMode("host"),
			Privileged:    true,
			RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
		},
		nil, nil, kloudknoxContainer,
	)
	if err != nil {
		return fmt.Errorf("install: container create: %w", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("install: container start: %w", err)
	}
	fmt.Println("Container kloudknox started.")

	return waitContainerRunning(ctx, cli, resp.ID, 30*time.Second)
}

func runUninstall(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	purge := fs.Bool("purge-policies", false, "also delete all KloudKnoxPolicy resources / policy files")
	if err := fs.Parse(args); err != nil {
		return err
	}

	env, err := resolvedEnv()
	if err != nil {
		return err
	}

	ctx := context.Background()
	switch env {
	case "k8s":
		return uninstallK8s(ctx, *purge)
	case "docker":
		return uninstallDocker(ctx, *purge)
	}
	return nil
}

func uninstallK8s(ctx context.Context, purge bool) error {
	kc, err := buildKubeClients()
	if err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}

	manifests := k8sManifests()
	for i := len(manifests) - 1; i >= 0; i-- {
		docs, _ := splitYAMLDocs(manifests[i])
		for j := len(docs) - 1; j >= 0; j-- {
			_ = deleteManifest(ctx, kc.Dynamic, kc.Mapper, docs[j])
		}
	}
	fmt.Println("KloudKnox resources deleted.")

	if purge {
		if err := deleteAllPolicies(ctx, kc); err != nil {
			fmt.Fprintf(os.Stderr, "warning: purge policies: %v\n", err)
		}
	}
	return nil
}

func uninstallDocker(ctx context.Context, purge bool) error {
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("uninstall: docker client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", kloudknoxContainer)),
	})
	if err != nil {
		return fmt.Errorf("uninstall: list containers: %w", err)
	}
	for _, c := range containers {
		_ = cli.ContainerStop(ctx, c.ID, container.StopOptions{})
		if err := cli.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true}); err != nil {
			return fmt.Errorf("uninstall: remove container: %w", err)
		}
		fmt.Printf("removed container %s\n", strings.Join(c.Names, ","))
	}

	_ = os.Remove(kloudknoxComposeFile)

	if purge {
		if err := os.RemoveAll(kloudknoxPolicyDir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: purge policies: %v\n", err)
		} else {
			fmt.Println("policy directory removed.")
		}
	}
	return nil
}

func runUpgrade(args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	img := fs.String("image", "", "new container image (required)")
	dryRun := fs.Bool("dry-run", false, "print what would change without applying")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *img == "" {
		return errors.New("upgrade: --image is required")
	}

	env, err := resolvedEnv()
	if err != nil {
		return err
	}

	ctx := context.Background()
	switch env {
	case "k8s":
		return upgradeK8s(ctx, *img, *dryRun)
	case "docker":
		return upgradeDocker(ctx, *img, *dryRun)
	}
	return nil
}

func upgradeK8s(ctx context.Context, img string, dryRun bool) error {
	kc, err := buildKubeClients()
	if err != nil {
		return fmt.Errorf("upgrade: %w", err)
	}
	ns := resolvedNamespace()

	ds, err := kc.Typed.AppsV1().DaemonSets(ns).Get(ctx, "kloudknox", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("upgrade: get daemonset: %w", err)
	}
	current := ""
	if len(ds.Spec.Template.Spec.Containers) > 0 {
		current = ds.Spec.Template.Spec.Containers[0].Image
	}
	fmt.Printf("current image: %s\n", current)
	fmt.Printf("new image:     %s\n", img)
	if dryRun {
		fmt.Println("(dry-run — no changes applied)")
		return nil
	}

	patch := fmt.Sprintf(`{"spec":{"template":{"spec":{"containers":[{"name":"kloudknox","image":%q}]}}}}`, img)
	_, err = kc.Typed.AppsV1().DaemonSets(ns).Patch(
		ctx, "kloudknox",
		k8stypes.StrategicMergePatchType,
		[]byte(patch),
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("upgrade: patch daemonset: %w", err)
	}
	fmt.Println("DaemonSet image updated.")
	return waitDaemonSet(ctx, kc, ns)
}

func upgradeDocker(ctx context.Context, img string, dryRun bool) error {
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("upgrade: docker client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	existing, err := cli.ContainerInspect(ctx, kloudknoxContainer)
	if err != nil {
		return fmt.Errorf("upgrade: container not found — run 'kkctl install' first")
	}
	fmt.Printf("current image: %s\n", existing.Config.Image)
	fmt.Printf("new image:     %s\n", img)
	if dryRun {
		fmt.Println("(dry-run — no changes applied)")
		return nil
	}

	reader, err := cli.ImagePull(ctx, img, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("upgrade: image pull: %w", err)
	}
	_, _ = io.Copy(io.Discard, reader)
	_ = reader.Close()

	_ = cli.ContainerStop(ctx, existing.ID, container.StopOptions{})
	_ = cli.ContainerRemove(ctx, existing.ID, container.RemoveOptions{Force: true})

	newCfg := *existing.Config
	newCfg.Image = img

	resp, err := cli.ContainerCreate(ctx,
		&newCfg,
		existing.HostConfig,
		nil, nil, kloudknoxContainer,
	)
	if err != nil {
		return fmt.Errorf("upgrade: container create: %w", err)
	}
	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("upgrade: container start: %w", err)
	}
	fmt.Println("Container restarted with new image.")
	return waitContainerRunning(ctx, cli, resp.ID, 30*time.Second)
}

// applyManifest applies a single YAML document using server-side apply.
func applyManifest(ctx context.Context, dyn dynamic.Interface, mapper meta.RESTMapper, raw []byte) error {
	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(raw, obj); err != nil {
		return err
	}
	if obj.Object == nil {
		return nil
	}
	gvk := obj.GroupVersionKind()
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return err
	}
	nri := dyn.Resource(mapping.Resource)
	var rif dynamic.ResourceInterface = nri
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		ns := obj.GetNamespace()
		if ns == "" {
			ns = resolvedNamespace()
		}
		rif = nri.Namespace(ns)
	}
	_, err = rif.Apply(ctx, obj.GetName(), obj, metav1.ApplyOptions{
		FieldManager: "kkctl",
		Force:        true,
	})
	return err
}

func deleteManifest(ctx context.Context, dyn dynamic.Interface, mapper meta.RESTMapper, raw []byte) error {
	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(raw, obj); err != nil {
		return err
	}
	if obj.Object == nil {
		return nil
	}
	gvk := obj.GroupVersionKind()
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return err
	}
	nri := dyn.Resource(mapping.Resource)
	var rif dynamic.ResourceInterface = nri
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		ns := obj.GetNamespace()
		if ns == "" {
			ns = resolvedNamespace()
		}
		rif = nri.Namespace(ns)
	}
	return rif.Delete(ctx, obj.GetName(), metav1.DeleteOptions{})
}

// deleteAllPolicies removes every KloudKnoxPolicy CRD instance across all namespaces.
func deleteAllPolicies(ctx context.Context, kc *kubeClients) error {
	gk := schema.GroupKind{Group: "security.boanlab.com", Kind: "KloudKnoxPolicy"}
	mapping, err := kc.Mapper.RESTMapping(gk)
	if err != nil {
		return err
	}
	list, err := kc.Dynamic.Resource(mapping.Resource).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, item := range list.Items {
		ns := item.GetNamespace()
		var ri dynamic.ResourceInterface
		if ns != "" {
			ri = kc.Dynamic.Resource(mapping.Resource).Namespace(ns)
		} else {
			ri = kc.Dynamic.Resource(mapping.Resource)
		}
		_ = ri.Delete(ctx, item.GetName(), metav1.DeleteOptions{})
	}
	fmt.Printf("deleted %d KloudKnoxPolicy resources\n", len(list.Items))
	return nil
}

func waitDaemonSet(ctx context.Context, kc *kubeClients, ns string) error {
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		ds, err := kc.Typed.AppsV1().DaemonSets(ns).Get(ctx, "kloudknox", metav1.GetOptions{})
		if err == nil &&
			ds.Status.DesiredNumberScheduled > 0 &&
			ds.Status.NumberReady == ds.Status.DesiredNumberScheduled {
			return nil
		}
		time.Sleep(3 * time.Second)
		fmt.Print(".")
	}
	return errors.New("timed out waiting for DaemonSet rollout")
}

func waitContainerRunning(ctx context.Context, cli *dockerclient.Client, id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	fmt.Print("Waiting for container...")
	for time.Now().Before(deadline) {
		info, err := cli.ContainerInspect(ctx, id)
		if err == nil && info.State != nil {
			if info.State.Running {
				fmt.Println(" ready.")
				return nil
			}
			if info.State.ExitCode != 0 && info.State.Status == "exited" {
				return fmt.Errorf("container exited with code %d", info.State.ExitCode)
			}
		}
		time.Sleep(time.Second)
		fmt.Print(".")
	}
	return fmt.Errorf("container %s not running within %v", id[:12], timeout)
}
