// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

type probeResult struct {
	name string
	ok   bool
	note string
}

func runProbe(args []string) error {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	results := collectProbe()
	printProbe(results)
	for _, r := range results {
		if !r.ok {
			return fmt.Errorf("pre-flight check failed — fix the issues above before running 'kkctl install'")
		}
	}
	return nil
}

func collectProbe() []probeResult {
	var results []probeResult

	var uts unix.Utsname
	if err := unix.Uname(&uts); err != nil {
		results = append(results, probeResult{"Kernel version", false, "uname failed: " + err.Error()})
	} else {
		release := byteArrayToString(uts.Release[:])
		ok, note := checkKernelVersion(release)
		results = append(results, probeResult{"Kernel version", ok, release + note})
	}

	if _, err := os.Stat("/sys/kernel/btf/vmlinux"); err == nil {
		results = append(results, probeResult{"BTF (vmlinux)", true, "/sys/kernel/btf/vmlinux found"})
	} else {
		results = append(results, probeResult{"BTF (vmlinux)", false, "not found — kernel may lack CONFIG_DEBUG_INFO_BTF"})
	}

	if data, err := os.ReadFile("/sys/fs/cgroup/cgroup.controllers"); err == nil && len(data) > 0 {
		results = append(results, probeResult{"cgroup v2", true, "unified hierarchy"})
	} else {
		results = append(results, probeResult{"cgroup v2", false, "not detected — mount cgroup2 or enable unified hierarchy"})
	}

	lsmData, err := os.ReadFile("/sys/kernel/security/lsm")
	if err != nil {
		results = append(results, probeResult{"Enforcer mode", false, "cannot read /sys/kernel/security/lsm: " + err.Error()})
	} else {
		hasBPF, hasAppArmor := false, false
		for _, lsm := range strings.Split(strings.TrimSpace(string(lsmData)), ",") {
			switch strings.TrimSpace(lsm) {
			case "bpf":
				hasBPF = true
			case "apparmor":
				hasAppArmor = true
			}
		}
		switch {
		case hasBPF:
			results = append(results, probeResult{"Enforcer mode", true, "bpf"})
		case hasAppArmor:
			results = append(results, probeResult{"Enforcer mode", true, "apparmor (bpf LSM not listed in /sys/kernel/security/lsm)"})
		default:
			results = append(results, probeResult{"Enforcer mode", false, "neither bpf nor apparmor in /sys/kernel/security/lsm"})
		}
	}

	type capCheck struct {
		name string
		bit  int
	}
	caps := []capCheck{
		{"CAP_SYS_ADMIN", 21},
		{"CAP_NET_ADMIN", 12},
		{"CAP_MAC_ADMIN", 33},
		{"CAP_BPF", 39},
		{"CAP_PERFMON", 38},
	}
	eff := readCapEff()
	for _, c := range caps {
		has := eff>>c.bit&1 == 1
		note := ""
		if !has {
			note = "missing — run as root or grant " + c.name
		}
		results = append(results, probeResult{c.name, has, note})
	}

	env, err := resolvedEnv()
	if err != nil {
		results = append(results, probeResult{"Environment", false, err.Error()})
	} else {
		results = append(results, probeResult{"Environment", true, env + " detected"})
	}

	return results
}

func printProbe(results []probeResult) {
	fmt.Println("KloudKnox Pre-flight Check")
	failed := 0
	for _, r := range results {
		mark := "✓"
		if !r.ok {
			mark = "✗"
			failed++
		}
		if r.note != "" {
			fmt.Printf("  [%s] %-20s %s\n", mark, r.name, r.note)
		} else {
			fmt.Printf("  [%s] %s\n", mark, r.name)
		}
	}
	fmt.Println()
	if failed == 0 {
		fmt.Println("All checks passed.")
	} else {
		fmt.Printf("%d check(s) failed.\n", failed)
	}
}

func checkKernelVersion(release string) (bool, string) {
	parts := strings.SplitN(release, ".", 3)
	if len(parts) < 2 {
		return false, " (could not parse)"
	}
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(strings.SplitN(parts[1], "-", 2)[0])
	if major > 5 || (major == 5 && minor >= 8) {
		return true, " (≥ 5.8 ✓)"
	}
	return false, fmt.Sprintf(" (need ≥ 5.8, got %d.%d)", major, minor)
}

func readCapEff() uint64 {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "CapEff:") {
			continue
		}
		hexStr := strings.TrimSpace(strings.TrimPrefix(line, "CapEff:"))
		b, err := hex.DecodeString(hexStr)
		if err != nil || len(b) == 0 {
			return 0
		}
		var val uint64
		for _, by := range b {
			val = val<<8 | uint64(by)
		}
		return val
	}
	return 0
}

// byteArrayToString converts a null-terminated byte slice (e.g. unix.Utsname fields) to a Go string.
func byteArrayToString(arr []byte) string {
	for i, v := range arr {
		if v == 0 {
			return string(arr[:i])
		}
	}
	return string(arr)
}
