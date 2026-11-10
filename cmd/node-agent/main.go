/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sdncontroller/sdncontroller/internal/agent"
	"github.com/sdncontroller/sdncontroller/internal/telemetry"
)

const (
	serviceName    = "sdn-node-agent"
	serviceVersion = "0.1.0"
)

func main() {
	// Parse command-line flags.
	var (
		nodeName     string
		kubeConfig   string
		otlpEndpoint string
		ovsSocket    string
	)
	flag.StringVar(&nodeName, "node-name", os.Getenv("NODE_NAME"), "Kubernetes node name (defaults to NODE_NAME env var)")
	flag.StringVar(&kubeConfig, "kubeconfig", "", "Path to kubeconfig file (defaults to in-cluster config)")
	flag.StringVar(&otlpEndpoint, "otlp-endpoint", "otel-collector:4317", "OpenTelemetry collector gRPC endpoint")
	flag.StringVar(&ovsSocket, "ovs-socket", "/var/run/openvswitch/db.sock", "Path to OVS database socket")
	flag.Parse()

	// Set up structured logging.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	if nodeName == "" {
		logger.Error("node-name is required (set --node-name or NODE_NAME env var)")
		os.Exit(1)
	}

	logger.Info("starting SDN node agent",
		"node", nodeName,
		"otlpEndpoint", otlpEndpoint,
		"ovsSocket", ovsSocket)

	// Set up signal handling for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Initialize OpenTelemetry.
	otelProvider, err := telemetry.Init(ctx, telemetry.Config{
		ServiceName:    serviceName,
		ServiceVersion: serviceVersion,
		OTLPEndpoint:   otlpEndpoint,
		MetricInterval: 15 * time.Second,
	})
	if err != nil {
		logger.Warn("failed to initialize OpenTelemetry, continuing without telemetry",
			"error", err)
	}

	// Create and start the agent.
	cfg := agent.Config{
		NodeName:     nodeName,
		KubeConfig:   kubeConfig,
		OTLPEndpoint: otlpEndpoint,
		OVSSocket:    ovsSocket,
	}

	nodeAgent := agent.New(cfg, logger)
	if err := nodeAgent.Start(ctx); err != nil {
		logger.Error("failed to start node agent", "error", err)
		os.Exit(1)
	}

	logger.Info("node agent running, waiting for shutdown signal")

	// Wait for shutdown signal.
	<-ctx.Done()
	logger.Info("shutdown signal received")

	// Graceful shutdown.
	nodeAgent.Stop()

	if otelProvider != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := otelProvider.Shutdown(shutdownCtx); err != nil {
			logger.Error("failed to shutdown OpenTelemetry", "error", err)
		}
	}

	logger.Info("node agent stopped")
}
