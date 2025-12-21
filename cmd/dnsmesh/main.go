package main

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/tommyskeff/dnsmesh/internal/cluster"
	"github.com/tommyskeff/dnsmesh/internal/config"
	"github.com/tommyskeff/dnsmesh/internal/dns"
	"github.com/tommyskeff/dnsmesh/internal/health"
	"github.com/tommyskeff/dnsmesh/internal/logging"
)

const (
	DNSPort       = ":53"
	HealthPort    = ":8080"
	DefaultTTL    = 30
	DefaultDomain = "clusterset.local"
)

func main() {
	logging.GetLevelFromEnv()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	secretName := os.Getenv("GLOBAL_CONFIG_SECRET")
	if secretName == "" {
		logging.Error("GLOBAL_CONFIG_SECRET environment variable is required")
		os.Exit(1)
	}

	ttl := getTTLFromEnv()
	domain := getDomainFromEnv()

	dnsTable := dns.NewTable()
	healthServer := health.NewServer()

	var clusterManager *cluster.Manager
	initialStartup := true

	onConfigChange := func(cfg *config.GlobalConfig) {
		logging.Info("Configuration loaded: %d clusters", len(cfg.Clusters))

		if initialStartup {
			// First load - just create the manager, InitialConnect will be called after
			dnsTable.Clear()
			clusterManager = cluster.NewManager(domain, dnsTable)
			return
		}

		// Subsequent config changes - update clusters
		if clusterManager != nil {
			clusterManager.Stop()
		}

		dnsTable.Clear()
		clusterManager = cluster.NewManager(domain, dnsTable)
		clusterManager.UpdateClusters(cfg.Clusters)
	}

	loader, err := config.NewLoader(secretName, onConfigChange)
	if err != nil {
		logging.Error("Error creating config loader: %v", err)
		os.Exit(1)
	}

	if err := loader.Start(ctx); err != nil {
		logging.Error("Error loading initial configuration: %v", err)
		os.Exit(1)
	}

	cfg := loader.GetConfig()
	if clusterManager != nil && cfg != nil {
		logging.Info("Performing initial connection to %d clusters...", len(cfg.Clusters))
		clusterManager.InitialConnect(cfg.Clusters)
		logging.Info("Initial connection attempts complete")
	}

	initialStartup = false

	dnsServer := dns.NewServer(dnsTable, ttl)

	go func() {
		if err := healthServer.Start(HealthPort); err != nil {
			logging.Error("Health server error: %v", err)
		}
	}()

	go func() {
		if err := dnsServer.Start(DNSPort); err != nil {
			logging.Error("DNS server error: %v", err)
			cancel()
		}
	}()

	healthServer.SetReady(true)
	logging.Info("DNSMesh started (domain=%s, ttl=%ds)", domain, ttl)

	select {
	case <-sigChan:
		logging.Info("Shutting down...")
	case <-ctx.Done():
	}

	healthServer.SetReady(false)

	if clusterManager != nil {
		clusterManager.Stop()
	}
	if err := dnsServer.Stop(); err != nil {
		logging.Error("Error stopping DNS server: %v", err)
	}
	if err := healthServer.Stop(); err != nil {
		logging.Error("Error stopping health server: %v", err)
	}

	logging.Info("DNSMesh service stopped")
}

func getTTLFromEnv() uint32 {
	ttlStr := os.Getenv("DNS_TTL")
	if ttlStr == "" {
		return DefaultTTL
	}

	ttl, err := strconv.ParseUint(ttlStr, 10, 32)
	if err != nil {
		logging.Warn("Invalid DNS_TTL value '%s', using default %d", ttlStr, DefaultTTL)
		return DefaultTTL
	}

	return uint32(ttl)
}

func getDomainFromEnv() string {
	domain := os.Getenv("DNS_DOMAIN")
	if domain == "" {
		return DefaultDomain
	}
	return domain
}
