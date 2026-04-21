// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

func runSysdump(args []string) error {
	fs := flag.NewFlagSet("sysdump", flag.ContinueOnError)
	out := fs.String("o", "", "output file (default: kloudknox-dump-<timestamp>.tar.gz)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *out == "" {
		*out = fmt.Sprintf("kloudknox-dump-%s.tar.gz", time.Now().Format("20060102-150405"))
	}

	env, _ := resolvedEnv()
	files := map[string][]byte{}

	probeOut := &bytes.Buffer{}
	for _, r := range collectProbe() {
		mark := "✓"
		if !r.ok {
			mark = "✗"
		}
		fmt.Fprintf(probeOut, "[%s] %-20s %s\n", mark, r.name, r.note)
	}
	files["probe.txt"] = probeOut.Bytes()
	files["status.txt"] = captureStatus(env)
	files["policies.yaml"] = capturePoliciesYAML(env)

	switch env {
	case "k8s":
		if b := captureK8sEvents(); len(b) > 0 {
			files["k8s-events.txt"] = b
		}
		if b := captureK8sPodLogs(); len(b) > 0 {
			files["server.log"] = b
		}
	case "docker":
		if b := captureDockerLogs(); len(b) > 0 {
			files["docker.log"] = b
		}
	}

	meta := map[string]string{
		"kkctl_version": Version,
		"collected_at":  time.Now().UTC().Format(time.RFC3339),
		"environment":   env,
	}
	if b, err := json.MarshalIndent(meta, "", "  "); err == nil {
		files["meta.json"] = b
	}

	if err := writeTarGz(*out, files); err != nil {
		return fmt.Errorf("sysdump: %w", err)
	}
	fmt.Printf("sysdump written to %s\n", *out)
	return nil
}

// captureStatus redirects stdout through an os.Pipe so the existing
// status printer can be reused without changing its signature.
func captureStatus(env string) []byte {
	var buf bytes.Buffer
	r, w, err := os.Pipe()
	if err != nil {
		fmt.Fprintf(&buf, "status capture failed: %v\n", err)
		return buf.Bytes()
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan struct{})
	go func() {
		_, _ = buf.ReadFrom(r)
		close(done)
	}()
	ctx := context.Background()
	var runErr error
	switch env {
	case "k8s":
		runErr = statusK8s(ctx, "table")
	case "docker":
		runErr = statusDocker(ctx, "table")
	}
	_ = w.Close()
	os.Stdout = saved
	<-done
	if runErr != nil {
		fmt.Fprintf(&buf, "\nstatus error: %v\n", runErr)
	}
	return buf.Bytes()
}

func capturePoliciesYAML(env string) []byte {
	switch env {
	case "k8s":
		return dumpK8sPolicies()
	case "docker":
		return dumpDockerPolicies()
	}
	return []byte("# no environment detected\n")
}

func dumpK8sPolicies() []byte {
	kc, err := buildKubeClients()
	if err != nil {
		return fmt.Appendf(nil, "# kube client error: %v\n", err)
	}
	gk := schema.GroupKind{Group: "security.boanlab.com", Kind: "KloudKnoxPolicy"}
	mapping, err := kc.Mapper.RESTMapping(gk)
	if err != nil {
		return fmt.Appendf(nil, "# RESTMapping error: %v\n", err)
	}
	list, err := kc.Dynamic.Resource(mapping.Resource).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return fmt.Appendf(nil, "# list error: %v\n", err)
	}
	var buf bytes.Buffer
	for i, item := range list.Items {
		if i > 0 {
			buf.WriteString("---\n")
		}
		b, err := yaml.Marshal(item.Object)
		if err != nil {
			continue
		}
		buf.Write(b)
	}
	return buf.Bytes()
}

func dumpDockerPolicies() []byte {
	entries, err := os.ReadDir(kloudknoxPolicyDir)
	if err != nil {
		return fmt.Appendf(nil, "# policy dir unreadable: %v\n", err)
	}
	var buf bytes.Buffer
	first := true
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(kloudknoxPolicyDir, e.Name())) // #nosec G304
		if err != nil {
			continue
		}
		if !first {
			buf.WriteString("---\n")
		}
		first = false
		buf.Write(data)
		if len(data) > 0 && data[len(data)-1] != '\n' {
			buf.WriteByte('\n')
		}
	}
	return buf.Bytes()
}

func captureK8sEvents() []byte {
	kc, err := buildKubeClients()
	if err != nil {
		return nil
	}
	ctx := context.Background()
	ns := resolvedNamespace()
	events, err := kc.Typed.CoreV1().Events(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}
	var buf bytes.Buffer
	for _, e := range events.Items {
		fmt.Fprintf(&buf, "%s  %s  %s/%s  %s\n",
			e.LastTimestamp.Format(time.RFC3339),
			e.Type, e.InvolvedObject.Kind, e.InvolvedObject.Name,
			e.Message)
	}
	return buf.Bytes()
}

func captureK8sPodLogs() []byte {
	kc, err := buildKubeClients()
	if err != nil {
		return nil
	}
	ctx := context.Background()
	ns := resolvedNamespace()
	pods, err := kc.Typed.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "boanlab.com/app=kloudknox",
	})
	if err != nil || len(pods.Items) == 0 {
		return nil
	}
	tail := int64(1000)
	var buf bytes.Buffer
	for _, p := range pods.Items {
		fmt.Fprintf(&buf, "===== pod/%s (node %s) =====\n", p.Name, p.Spec.NodeName)
		req := kc.Typed.CoreV1().Pods(ns).GetLogs(p.Name, &corev1.PodLogOptions{TailLines: &tail})
		stream, err := req.Stream(ctx)
		if err != nil {
			fmt.Fprintf(&buf, "log stream error: %v\n", err)
			continue
		}
		_, _ = buf.ReadFrom(stream)
		_ = stream.Close()
	}
	return buf.Bytes()
}

func captureDockerLogs() []byte {
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return nil
	}
	defer func() { _ = cli.Close() }()

	ctx := context.Background()
	cs, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", kloudknoxContainer)),
	})
	if err != nil || len(cs) == 0 {
		return nil
	}

	tail := "1000"
	reader, err := cli.ContainerLogs(ctx, cs[0].ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
	})
	if err != nil {
		return nil
	}
	defer func() { _ = reader.Close() }()
	var buf bytes.Buffer
	// Docker multiplexes stdout/stderr with 8-byte headers; stdcopy strips them.
	_, _ = stdcopy.StdCopy(&buf, &buf, reader)
	return buf.Bytes()
}

func writeTarGz(dst string, files map[string][]byte) (retErr error) {
	f, err := os.Create(dst) // #nosec G304
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && retErr == nil {
			retErr = cerr
		}
	}()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	for name, data := range files {
		hdr := &tar.Header{
			Name:    name,
			Size:    int64(len(data)),
			Mode:    0o644,
			ModTime: time.Now(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gw.Close()
}
