// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 BoanLab @ Dankook University

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/boanlab/KloudKnox/protobuf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Filter holds the fields used to select which events or alerts the server
// streams to the client. An empty field matches everything.
type Filter struct {
	EventName string

	Source    string
	Category  string
	Operation string
	Resource  string
	Data      string

	NodeName      string
	NamespaceName string
	PodName       string
	ContainerName string
	Labels        string
}

// EventData is the JSON shape emitted on stdout for each streamed event.
type EventData struct {
	Timestamp uint64 `json:"timestamp"`
	CPUID     uint32 `json:"cpuID"`
	SeqNum    uint32 `json:"seqNum"`

	HostPPID int32 `json:"hostPPID"`
	HostPID  int32 `json:"hostPID"`
	HostTID  int32 `json:"hostTID"`

	PPID int32 `json:"PPID"`
	PID  int32 `json:"PID"`
	TID  int32 `json:"TID"`

	UID uint32 `json:"UID"`
	GID uint32 `json:"GID"`

	EventID   int32  `json:"eventID"`
	EventName string `json:"eventName"`
	RetVal    int32  `json:"retVal"`
	RetCode   string `json:"retCode"`

	Source    string `json:"source"`
	Category  string `json:"category"`
	Operation string `json:"operation"`
	Resource  string `json:"resource"`
	Data      string `json:"data"`

	NodeName      string `json:"nodeName"`
	NamespaceName string `json:"namespaceName,omitempty"`
	PodName       string `json:"podName,omitempty"`
	ContainerName string `json:"containerName,omitempty"`
	Labels        string `json:"labels,omitempty"`
}

// AlertData is the JSON shape emitted on stdout for each streamed alert.
type AlertData struct {
	Timestamp uint64 `json:"timestamp"`
	CPUID     uint32 `json:"cpuID"`
	SeqNum    uint32 `json:"seqNum"`

	HostPPID int32 `json:"hostPPID"`
	HostPID  int32 `json:"hostPID"`
	HostTID  int32 `json:"hostTID"`

	PPID int32 `json:"PPID"`
	PID  int32 `json:"PID"`
	TID  int32 `json:"TID"`

	UID uint32 `json:"UID"`
	GID uint32 `json:"GID"`

	EventID   int32  `json:"eventID"`
	EventName string `json:"eventName"`
	RetVal    int32  `json:"retVal"`
	RetCode   string `json:"retCode"`

	Source    string `json:"source"`
	Category  string `json:"category"`
	Operation string `json:"operation"`
	Resource  string `json:"resource"`
	Data      string `json:"data"`

	NodeName      string `json:"nodeName"`
	NamespaceName string `json:"namespaceName,omitempty"`
	PodName       string `json:"podName,omitempty"`
	ContainerName string `json:"containerName,omitempty"`
	Labels        string `json:"labels,omitempty"`

	PolicyName   string `json:"policyName"`
	PolicyAction string `json:"policyAction"`
}

var streamEnc = json.NewEncoder(os.Stdout)

func runStream(args []string) error {
	if len(args) == 0 {
		return errors.New("stream: usage: kkctl stream events|alerts|logs [flags]")
	}
	target := args[0]
	switch target {
	case "events", "event":
		target = "event"
	case "alerts", "alert":
		target = "alert"
	case "logs", "log":
		return runStreamLogs(args[1:])
	default:
		return fmt.Errorf("stream: unknown target %q (expected events|alerts|logs)", target)
	}

	fs := flag.NewFlagSet("stream "+target, flag.ContinueOnError)

	server := fs.String("server", "", "gRPC server host:port (overrides ~/.kkctl.yaml)")
	eventName := fs.String("eventName", "", "filter by event name")
	source := fs.String("source", "", "filter by source")
	category := fs.String("category", "", "filter by category")
	operation := fs.String("operation", "", "filter by operation")
	resource := fs.String("resource", "", "filter by resource")
	data := fs.String("data", "", "filter by data (substring)")
	nodeName := fs.String("nodeName", "", "filter by node name")
	namespaceName := fs.String("namespaceName", "", "filter by namespace name")
	podName := fs.String("podName", "", "filter by pod name prefix")
	containerName := fs.String("containerName", "", "filter by container name prefix")
	labels := fs.String("labels", "", "filter by labels (substring)")

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	cfg := loadClientConfig()
	if *server != "" {
		cfg.Server = *server
	}

	filter := Filter{
		EventName:     *eventName,
		Source:        *source,
		Category:      *category,
		Operation:     *operation,
		Resource:      *resource,
		Data:          *data,
		NodeName:      *nodeName,
		NamespaceName: *namespaceName,
		PodName:       *podName,
		ContainerName: *containerName,
		Labels:        *labels,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nShutting down...")
		cancel()
	}()

	runClient(ctx, cfg.Server, target, filter)
	return nil
}

func runClient(ctx context.Context, serverAddr, target string, filter Filter) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := connectAndStream(ctx, serverAddr, target, filter); err != nil {
			fmt.Fprintf(os.Stderr, "Connection lost: %v\nReconnecting in 5 seconds...\n", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func connectAndStream(ctx context.Context, serverAddr, target string, filter Filter) error {
	conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", serverAddr, err)
	}
	defer func() { _ = conn.Close() }()

	client := protobuf.NewKloudKnoxClient(conn)
	fmt.Fprintf(os.Stderr, "Connected to %s\n", serverAddr)

	if target == "alert" {
		return streamAlerts(ctx, client, filter)
	}
	return streamEvents(ctx, client, filter)
}

func streamEvents(ctx context.Context, client protobuf.KloudKnoxClient, f Filter) error {
	stream, err := client.EventStream(ctx, &protobuf.EventFilter{
		EventName:     f.EventName,
		Source:        f.Source,
		Category:      f.Category,
		Operation:     f.Operation,
		Resource:      f.Resource,
		Data:          f.Data,
		NodeName:      f.NodeName,
		NamespaceName: f.NamespaceName,
		PodName:       f.PodName,
		ContainerName: f.ContainerName,
		Labels:        f.Labels,
	})
	if err != nil {
		return fmt.Errorf("failed to open event stream: %w", err)
	}
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			fmt.Fprintln(os.Stderr, "Stream closed by server")
			return nil
		}
		if err != nil {
			return fmt.Errorf("event stream error: %w", err)
		}
		writeJSONStream(EventData{
			Timestamp:     ev.Timestamp,
			CPUID:         ev.CPUID,
			SeqNum:        ev.SeqNum,
			HostPPID:      ev.HostPPID,
			HostPID:       ev.HostPID,
			HostTID:       ev.HostTID,
			PPID:          ev.PPID,
			PID:           ev.PID,
			TID:           ev.TID,
			UID:           ev.UID,
			GID:           ev.GID,
			EventID:       ev.EventID,
			EventName:     ev.EventName,
			RetVal:        ev.RetVal,
			RetCode:       ev.RetCode,
			Source:        ev.Source,
			Category:      ev.Category,
			Operation:     ev.Operation,
			Resource:      ev.Resource,
			Data:          ev.Data,
			NodeName:      ev.NodeName,
			NamespaceName: ev.NamespaceName,
			PodName:       ev.PodName,
			ContainerName: ev.ContainerName,
			Labels:        ev.Labels,
		})
	}
}

func streamAlerts(ctx context.Context, client protobuf.KloudKnoxClient, f Filter) error {
	stream, err := client.AlertStream(ctx, &protobuf.AlertFilter{
		EventName:     f.EventName,
		Source:        f.Source,
		Category:      f.Category,
		Operation:     f.Operation,
		Resource:      f.Resource,
		Data:          f.Data,
		NodeName:      f.NodeName,
		NamespaceName: f.NamespaceName,
		PodName:       f.PodName,
		ContainerName: f.ContainerName,
		Labels:        f.Labels,
	})
	if err != nil {
		return fmt.Errorf("failed to open alert stream: %w", err)
	}
	for {
		at, err := stream.Recv()
		if err == io.EOF {
			fmt.Fprintln(os.Stderr, "Stream closed by server")
			return nil
		}
		if err != nil {
			return fmt.Errorf("alert stream error: %w", err)
		}
		writeJSONStream(AlertData{
			Timestamp:     at.Timestamp,
			CPUID:         at.CPUID,
			SeqNum:        at.SeqNum,
			HostPPID:      at.HostPPID,
			HostPID:       at.HostPID,
			HostTID:       at.HostTID,
			PPID:          at.PPID,
			PID:           at.PID,
			TID:           at.TID,
			UID:           at.UID,
			GID:           at.GID,
			EventID:       at.EventID,
			EventName:     at.EventName,
			RetVal:        at.RetVal,
			RetCode:       at.RetCode,
			Source:        at.Source,
			Category:      at.Category,
			Operation:     at.Operation,
			Resource:      at.Resource,
			Data:          at.Data,
			NodeName:      at.NodeName,
			NamespaceName: at.NamespaceName,
			PodName:       at.PodName,
			ContainerName: at.ContainerName,
			Labels:        at.Labels,
			PolicyName:    at.PolicyName,
			PolicyAction:  at.PolicyAction,
		})
	}
}

func writeJSONStream(v any) {
	if err := streamEnc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
	}
}

// LogData is the JSON shape emitted on stdout for each streamed log entry.
type LogData struct {
	Timestamp uint64 `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}

func runStreamLogs(args []string) error {
	fs := flag.NewFlagSet("stream logs", flag.ContinueOnError)
	server := fs.String("server", "", "gRPC server host:port (overrides ~/.kkctl.yaml)")
	level := fs.String("level", "", "filter by log level (e.g. INFO, WARN, ERROR)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := loadClientConfig()
	if *server != "" {
		cfg.Server = *server
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\nShutting down...")
		cancel()
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if err := connectAndStreamLogs(ctx, cfg.Server, *level); err != nil {
			fmt.Fprintf(os.Stderr, "Connection lost: %v\nReconnecting in 5 seconds...\n", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func connectAndStreamLogs(ctx context.Context, serverAddr, level string) error {
	conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", serverAddr, err)
	}
	defer func() { _ = conn.Close() }()

	client := protobuf.NewKloudKnoxClient(conn)
	fmt.Fprintf(os.Stderr, "Connected to %s\n", serverAddr)

	stream, err := client.LogStream(ctx, &protobuf.LogFilter{Level: level})
	if err != nil {
		return fmt.Errorf("failed to open log stream: %w", err)
	}
	for {
		entry, err := stream.Recv()
		if err == io.EOF {
			fmt.Fprintln(os.Stderr, "Stream closed by server")
			return nil
		}
		if err != nil {
			return fmt.Errorf("log stream error: %w", err)
		}
		writeJSONStream(LogData{
			Timestamp: entry.Timestamp,
			Level:     entry.Level,
			Message:   entry.Message,
		})
	}
}
