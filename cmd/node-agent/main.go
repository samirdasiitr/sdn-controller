/*
Copyright (c) 2026 Samir Das. All rights reserved.

PROPRIETARY AND CONFIDENTIAL. No license is granted. Any use of this software, and in
particular production or commercial use, requires the prior explicit written permission of
Samir Das <samiruor@gmail.com>. See the LICENSE file.
*/

package main

import (
	"encoding/json"
	"regexp"

	"github.com/sdncontroller/sdncontroller/internal/agent/ovsinfo"
	"net/http"

	"context"
	"flag"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log/slog"
	"os"
	"os/signal"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
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
		metricsAddr  string
	)
	flag.StringVar(&nodeName, "node-name", os.Getenv("NODE_NAME"), "Kubernetes node name (defaults to NODE_NAME env var)")
	flag.StringVar(&kubeConfig, "kubeconfig", "", "Path to kubeconfig file (defaults to in-cluster config)")
	flag.StringVar(&otlpEndpoint, "otlp-endpoint", "otel-collector:4317", "OpenTelemetry collector gRPC endpoint")
	flag.StringVar(&ovsSocket, "ovs-socket", "/var/run/openvswitch/db.sock", "Path to OVS database socket")
	flag.StringVar(&metricsAddr, "metrics-addr", ":9101", "Address to serve Prometheus metrics on (empty disables)")
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

	if metricsAddr != "" {
		go serveMetrics(ctx, logger, metricsAddr, nodeName)
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

// serveMetrics exposes per-node OVS state (bridges, ports, flows, throughput) for Prometheus.
func serveMetrics(ctx context.Context, logger *slog.Logger, addr, nodeName string) {
	collector := telemetry.NewOVSCollector(10*time.Second, nodeName, false)
	go func() { _ = collector.Start(logr.NewContext(ctx, logr.Discard())) }()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(ctrlmetrics.Registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/ovs/topology", func(w http.ResponseWriter, r *http.Request) {
		t, err := ovsinfo.GetTopology(r.Context(), nodeName)
		writeJSON(w, t, err)
	})
	mux.HandleFunc("/ovs/flows", func(w http.ResponseWriter, r *http.Request) {
		br := r.URL.Query().Get("bridge")
		if !bridgeNameRe.MatchString(br) {
			http.Error(w, `{"error":"invalid bridge name"}`, http.StatusBadRequest)
			return
		}
		f, err := ovsinfo.GetFlows(r.Context(), br)
		writeJSON(w, map[string]interface{}{"bridge": br, "node": nodeName, "flows": f}, err)
	})
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	logger.Info("serving metrics", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("metrics server failed", "error", err)
	}
}

var bridgeNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,14}$`)

func writeJSON(w http.ResponseWriter, v interface{}, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}
