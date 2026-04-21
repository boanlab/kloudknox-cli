// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// Version is the CLI version string shown by `kkctl version`.
// Release builds set it via `-ldflags "-X cmd.Version=<tag>"`.
var Version = "dev"

// Execute is the kkctl entry point. It parses command-line arguments,
// runs the requested subcommand, and exits with an appropriate status code.
//
// Usage:
//
//	kkctl [global flags] <command> [command flags]
func Execute() {
	gfs := flag.NewFlagSet("kkctl", flag.ContinueOnError)
	gfs.SetOutput(io.Discard)

	gfs.StringVar(&globalCfg.Env, "env", "", "target environment: k8s|docker (default: auto-detect)")
	gfs.StringVar(&globalCfg.KubeConfig, "kubeconfig", "", "path to kubeconfig file (default: ~/.kube/config)")
	gfs.StringVar(&globalCfg.KubeContext, "kube-context", "", "kubeconfig context name")
	gfs.StringVar(&globalCfg.Namespace, "namespace", defaultNS, "Kubernetes namespace (default: kloudknox)")
	gfs.StringVar(&globalCfg.Server, "server", "", "gRPC server host:port (default: "+defaultServer+")")
	gfs.StringVar(&globalCfg.Output, "o", "table", "output format: table|wide|json|yaml")

	_ = gfs.Parse(os.Args[1:])

	rest := gfs.Args()
	if len(rest) == 0 {
		printUsage()
		os.Exit(2)
	}

	verb := rest[0]
	args := rest[1:]

	var err error
	switch verb {
	case "probe":
		err = runProbe(args)
	case "install":
		err = runInstall(args)
	case "uninstall":
		err = runUninstall(args)
	case "upgrade":
		err = runUpgrade(args)
	case "status":
		err = runStatus(args)
	case "apply":
		err = runApply(args)
	case "delete":
		err = runDelete(args)
	case "get":
		err = runGet(args)
	case "describe":
		err = runDescribe(args)
	case "label":
		err = runLabel(args)
	case "inject":
		err = runInject(args)
	case "policy":
		err = runPolicy(args)
	case "stream":
		err = runStream(args)
	case "sysdump":
		err = runSysdump(args)
	case "completion":
		err = runCompletion(args)
	case "version":
		err = runVersion(args)
	case "-h", "--help", "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", verb)
		printUsage()
		os.Exit(2)
	}

	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runCompletion(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("completion: specify a shell: bash|zsh")
	}
	switch args[0] {
	case "bash":
		_, err := os.Stdout.Write(completionBash)
		return err
	case "zsh":
		_, err := os.Stdout.Write(completionZsh)
		return err
	default:
		return fmt.Errorf("completion: unsupported shell %q (supported: bash, zsh)", args[0])
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `kkctl — KloudKnox CLI

Global flags (place before the verb):
  --env k8s|docker       Target environment (default: auto-detect)
  --kubeconfig <path>    Path to kubeconfig file (default: ~/.kube/config)
  --kube-context <name>  kubeconfig context name
  --namespace <ns>       Kubernetes namespace (default: kloudknox)
  --server <addr>        gRPC server host:port (default: localhost:36890)
  -o table|wide|json|yaml  Output format (default: table)

Commands:
  probe                             Pre-flight environment check
  install   [--image <img>] [--wait=true] [--skip-apparmor-webhook]  Deploy KloudKnox
  uninstall [--purge-policies]      Remove KloudKnox
  upgrade   --image <img> [--dry-run]                              Upgrade KloudKnox image
  status                            Show KloudKnox status
  apply     -f <file>               Upsert policies from YAML
  delete    policy <name>... | -f <file>  Delete one or more policies
  get       policies|nodes            List resources
  describe  policy|container|node <name>  Show resource detail
  label     container|pod <name> <key=val>  Inject labels
  inject    -f <policy> --target container|pod  Inject policy labels
  policy    validate -f <file>      Validate policy YAML offline
  stream    events|alerts|logs [flags]  Stream over gRPC
  sysdump   [-o <file.tar.gz>]      Collect debug bundle
  completion bash|zsh               Print shell completion script
  version                           Print kkctl version

Use "kkctl <command> -h" for per-command flags.
`)
}
