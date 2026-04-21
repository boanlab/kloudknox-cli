// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"
)

// TestFilterFields verifies Filter struct holds all fields.
func TestFilterFields(t *testing.T) {
	f := Filter{
		EventName:     "execve",
		Source:        "process",
		Category:      "file",
		Operation:     "read",
		Resource:      "/etc/passwd",
		Data:          "some data",
		NodeName:      "node-1",
		NamespaceName: "production",
		PodName:       "web-pod",
		ContainerName: "app",
		Labels:        "app=web",
	}

	if f.EventName != "execve" {
		t.Errorf("EventName = %q, want execve", f.EventName)
	}
	if f.Source != "process" {
		t.Errorf("Source = %q, want process", f.Source)
	}
	if f.Category != "file" {
		t.Errorf("Category = %q, want file", f.Category)
	}
	if f.Operation != "read" {
		t.Errorf("Operation = %q, want read", f.Operation)
	}
	if f.Resource != "/etc/passwd" {
		t.Errorf("Resource = %q, want /etc/passwd", f.Resource)
	}
	if f.Data != "some data" {
		t.Errorf("Data = %q, want some data", f.Data)
	}
	if f.NodeName != "node-1" {
		t.Errorf("NodeName = %q, want node-1", f.NodeName)
	}
	if f.NamespaceName != "production" {
		t.Errorf("NamespaceName = %q, want production", f.NamespaceName)
	}
	if f.PodName != "web-pod" {
		t.Errorf("PodName = %q, want web-pod", f.PodName)
	}
	if f.ContainerName != "app" {
		t.Errorf("ContainerName = %q, want app", f.ContainerName)
	}
	if f.Labels != "app=web" {
		t.Errorf("Labels = %q, want app=web", f.Labels)
	}
}

// TestFilterEmpty verifies zero-value Filter fields are empty strings.
func TestFilterEmpty(t *testing.T) {
	f := Filter{}
	if f.EventName != "" || f.Source != "" || f.Category != "" {
		t.Error("Expected empty Filter to have all zero-value string fields")
	}
}

// TestWriteJSONStreamEvent verifies writeJSONStream encodes an EventData to the global encoder.
func TestWriteJSONStreamEvent(t *testing.T) {
	var buf bytes.Buffer
	streamEnc = json.NewEncoder(&buf)

	writeJSONStream(EventData{
		EventName: "execve",
		PodName:   "web-pod",
		PID:       1234,
	})

	var got EventData
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("Failed to unmarshal output: %v", err)
	}
	if got.EventName != "execve" {
		t.Errorf("EventName = %q, want execve", got.EventName)
	}
	if got.PodName != "web-pod" {
		t.Errorf("PodName = %q, want web-pod", got.PodName)
	}
	if got.PID != 1234 {
		t.Errorf("PID = %d, want 1234", got.PID)
	}
}

// TestWriteJSONStreamAlert verifies writeJSONStream encodes an AlertData.
func TestWriteJSONStreamAlert(t *testing.T) {
	var buf bytes.Buffer
	streamEnc = json.NewEncoder(&buf)

	writeJSONStream(AlertData{
		EventName:    "connect",
		PolicyName:   "my-policy",
		PolicyAction: "Block",
	})

	var got AlertData
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("Failed to unmarshal output: %v", err)
	}
	if got.EventName != "connect" {
		t.Errorf("EventName = %q, want connect", got.EventName)
	}
	if got.PolicyName != "my-policy" {
		t.Errorf("PolicyName = %q, want my-policy", got.PolicyName)
	}
	if got.PolicyAction != "Block" {
		t.Errorf("PolicyAction = %q, want Block", got.PolicyAction)
	}
}

// TestWriteJSONStreamMultiple verifies multiple newline-delimited writes decode cleanly.
func TestWriteJSONStreamMultiple(t *testing.T) {
	var buf bytes.Buffer
	streamEnc = json.NewEncoder(&buf)

	writeJSONStream(EventData{EventName: "execve"})
	writeJSONStream(EventData{EventName: "openat"})

	dec := json.NewDecoder(&buf)
	var got1, got2 EventData
	if err := dec.Decode(&got1); err != nil {
		t.Fatalf("Failed to decode first object: %v", err)
	}
	if err := dec.Decode(&got2); err != nil {
		t.Fatalf("Failed to decode second object: %v", err)
	}
	if got1.EventName != "execve" {
		t.Errorf("First EventName = %q, want execve", got1.EventName)
	}
	if got2.EventName != "openat" {
		t.Errorf("Second EventName = %q, want openat", got2.EventName)
	}
}

// TestWriteJSONStreamError verifies writeJSONStream handles encoder errors gracefully.
func TestWriteJSONStreamError(t *testing.T) {
	streamEnc = json.NewEncoder(&failWriter{})
	writeJSONStream(EventData{EventName: "execve"})
}

// TestEventDataJSONFields verifies JSON field names are serialized correctly.
func TestEventDataJSONFields(t *testing.T) {
	var buf bytes.Buffer
	streamEnc = json.NewEncoder(&buf)

	writeJSONStream(EventData{
		Timestamp: 123456789,
		CPUID:     2,
		SeqNum:    42,
		HostPPID:  100,
		HostPID:   200,
		HostTID:   300,
		PPID:      10,
		PID:       20,
		TID:       30,
		UID:       1000,
		GID:       1000,
		EventID:   59,
		EventName: "execve",
		RetVal:    0,
		RetCode:   "SUCCESS",
		Source:    "process",
		Category:  "execution",
		Operation: "exec",
		Resource:  "/bin/ls",
		Data:      "-la",
		NodeName:  "node-1",
	})

	var raw map[string]any
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	expectedKeys := []string{
		"timestamp", "cpuID", "seqNum", "hostPPID", "hostPID", "hostTID",
		"PPID", "PID", "TID", "UID", "GID", "eventID", "eventName",
		"retVal", "retCode", "source", "category", "operation", "resource",
		"data", "nodeName",
	}
	for _, key := range expectedKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("Expected JSON key %q to be present", key)
		}
	}
}

// TestAlertDataJSONFieldsPolicyInfo verifies policy fields are in the JSON output.
func TestAlertDataJSONFieldsPolicyInfo(t *testing.T) {
	var buf bytes.Buffer
	streamEnc = json.NewEncoder(&buf)

	writeJSONStream(AlertData{
		EventName:    "execve",
		PolicyName:   "deny-shells",
		PolicyAction: "Block",
	})

	var raw map[string]any
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if raw["policyName"] != "deny-shells" {
		t.Errorf("policyName = %v, want deny-shells", raw["policyName"])
	}
	if raw["policyAction"] != "Block" {
		t.Errorf("policyAction = %v, want Block", raw["policyAction"])
	}
}

// TestEventDataOmitEmptyContainerFields verifies optional container fields are omitted when empty.
func TestEventDataOmitEmptyContainerFields(t *testing.T) {
	var buf bytes.Buffer
	streamEnc = json.NewEncoder(&buf)

	writeJSONStream(EventData{
		EventName: "execve",
		NodeName:  "node-1",
	})

	var raw map[string]any
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if _, ok := raw["podName"]; ok {
		t.Error("Expected podName to be omitted when empty")
	}
	if _, ok := raw["containerName"]; ok {
		t.Error("Expected containerName to be omitted when empty")
	}
}

// TestAlertDataOmitEmptyContainerFields verifies optional container fields are omitted in alerts.
func TestAlertDataOmitEmptyContainerFields(t *testing.T) {
	var buf bytes.Buffer
	streamEnc = json.NewEncoder(&buf)

	writeJSONStream(AlertData{
		EventName: "execve",
		NodeName:  "node-1",
	})

	var raw map[string]any
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if _, ok := raw["podName"]; ok {
		t.Error("Expected podName to be omitted when empty")
	}
	if _, ok := raw["namespaceName"]; ok {
		t.Error("Expected namespaceName to be omitted when empty")
	}
}

// TestRunClientContextAlreadyCancelled verifies runClient returns immediately on a pre-cancelled context.
func TestRunClientContextAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		runClient(ctx, "localhost:36890", "event", Filter{})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runClient did not return promptly on pre-cancelled context")
	}
}

// TestRunClientReconnectInterruptedByContext verifies runClient exits the 5-second reconnect
// wait early when the context is cancelled.
func TestRunClientReconnectInterruptedByContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	runClient(ctx, "localhost:1", "event", Filter{})

	if elapsed := time.Since(start); elapsed >= 5*time.Second {
		t.Errorf("runClient waited full 5s reconnect delay; context cancellation should have interrupted it")
	}
}

// TestConnectAndStreamEventTargetReturnsError verifies connectAndStream returns an error
// and uses the event stream when target is not "alert".
func TestConnectAndStreamEventTargetReturnsError(t *testing.T) {
	err := connectAndStream(context.Background(), "localhost:1", "event", Filter{})
	if err == nil {
		t.Error("Expected error connecting to unreachable server, got nil")
	}
}

// TestConnectAndStreamAlertTargetReturnsError verifies connectAndStream returns an error
// and uses the alert stream when target is "alert".
func TestConnectAndStreamAlertTargetReturnsError(t *testing.T) {
	err := connectAndStream(context.Background(), "localhost:1", "alert", Filter{})
	if err == nil {
		t.Error("Expected error connecting to unreachable server, got nil")
	}
}

// TestConnectAndStreamContextCancelled verifies connectAndStream respects context cancellation.
func TestConnectAndStreamContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := connectAndStream(ctx, "localhost:1", "event", Filter{})
	if err == nil {
		t.Error("Expected error on cancelled context, got nil")
	}
}

// failWriter is an io.Writer that always returns an error.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
