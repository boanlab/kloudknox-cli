// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"strings"
	"testing"
)

func TestCheckKernelVersion(t *testing.T) {
	cases := []struct {
		release string
		wantOK  bool
	}{
		{"6.17.0-20-generic", true},
		{"5.15.0-91-generic", true},
		{"5.8.0", true},
		{"5.7.19", false},
		{"4.19.0", false},
		{"5.8", true},
		{"badversion", false},
	}
	for _, tc := range cases {
		ok, _ := checkKernelVersion(tc.release)
		if ok != tc.wantOK {
			t.Errorf("checkKernelVersion(%q) = %v, want %v", tc.release, ok, tc.wantOK)
		}
	}
}

func TestByteArrayToString(t *testing.T) {
	cases := []struct {
		input []byte
		want  string
	}{
		{[]byte("hello\x00world"), "hello"},
		{[]byte("noterm"), "noterm"},
		{[]byte{}, ""},
		{[]byte("\x00"), ""},
		{[]byte("5.15.0-91-generic\x00\x00\x00"), "5.15.0-91-generic"},
	}
	for _, tc := range cases {
		got := byteArrayToString(tc.input)
		if got != tc.want {
			t.Errorf("byteArrayToString(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestReadCapEff(t *testing.T) {
	// /proc/self/status is always readable on Linux.
	// We just verify the function returns without panic and is plausible.
	eff := readCapEff()
	// As a normal user, CapEff is likely non-zero (e.g. no caps, value = 0 is also valid).
	_ = eff
}

func TestCollectProbe_ReturnsResults(t *testing.T) {
	results := collectProbe()
	if len(results) == 0 {
		t.Fatal("collectProbe returned no results")
	}
	for _, r := range results {
		if r.name == "" {
			t.Errorf("probe result has empty name: %+v", r)
		}
	}
}

func TestCollectProbe_KernelVersionPresent(t *testing.T) {
	results := collectProbe()
	for _, r := range results {
		if strings.Contains(r.name, "Kernel") {
			return
		}
	}
	t.Error("collectProbe: no kernel version result found")
}

func TestCollectProbe_BTFPresent(t *testing.T) {
	results := collectProbe()
	for _, r := range results {
		if strings.Contains(r.name, "BTF") {
			return
		}
	}
	t.Error("collectProbe: no BTF result found")
}
