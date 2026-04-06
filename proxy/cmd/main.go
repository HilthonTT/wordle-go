package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hilthontt/wordle-go/proxy/internal/balancer"
	"github.com/hilthontt/wordle-go/proxy/internal/config"
	"github.com/hilthontt/wordle-go/proxy/internal/middleware"
	"github.com/hilthontt/wordle-go/proxy/internal/throttling"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func main() {
	// Define command line flags
	configPath := flag.String("config", "", "Path to configuration file")
	listenAddr := flag.String("listen", ":5004", "Address to listen on")
	strategyStr := flag.String("strategy", "round_robin", "Load balancing strategy")
	healthCheckInterval := flag.Duration("health-check-interval", 30*time.Second, "Health check interval")
	maxFailCount := flag.Int("max-fail-count", 3, "Maximum failure count before marking backend as down")

	flag.Parse()

	var cfg config.Config

	// If config file is provided, load it
	if *configPath != "" {
		data, err := os.ReadFile(*configPath)
		if err != nil {
			log.Fatalf("Error reading config file: %v", err)
		}

		if err := json.Unmarshal(data, &cfg); err != nil {
			log.Fatalf("Error parsing config file: %v", err)
		}
	} else {
		// Use command line flags
		cfg = config.Config{
			ListenAddr:          *listenAddr,
			HealthCheckInterval: *healthCheckInterval,
			MaxFailCount:        *maxFailCount,
			Strategy:            *strategyStr,
			Backends: []config.BackendConfig{
				{URL: "http://localhost:8080", Weight: 1},
			},
		}
	}

	// Parse strategy
	strategy, err := config.ParseStrategyString(cfg.Strategy)
	if err != nil {
		log.Fatalf("Invalid strategy: %v", err)
	}

	// Extract backends and weights
	backendURLs := make([]string, len(cfg.Backends))
	weights := make([]int, len(cfg.Backends))

	for i, backend := range cfg.Backends {
		backendURLs[i] = backend.URL
		weights[i] = backend.Weight
	}

	metrics := balancer.NewMetrics("loadBalancer")

	redisClient := redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName: "mymaster",
		SentinelAddrs: []string{
			"127.0.0.1:36379",
			"127.0.0.1:36380",
			"127.0.0.1:36381",
		},
		Password:         "password",
		SentinelPassword: "password",
		// Sentinel reports internal Docker IPs (172.18.x.x) for the master.
		// Redirect them to localhost since ports are mapped to the host.
		Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			return (&net.Dialer{}).DialContext(ctx, "tcp4", "127.0.0.1:"+port)
		},
	})
	globalCfg := throttling.ThrottleConfig{MaxRequests: 10_000, Interval: time.Minute, Spans: 6, Cooldown: 30 * time.Second}
	ipCfg := throttling.ThrottleConfig{MaxRequests: 100, Interval: time.Minute, Spans: 6, Cooldown: 30 * time.Second}

	ht := throttling.NewHierarchicalThrottler(
		&throttling.ThrottleLevel{
			Name:         "global",
			KeyExtractor: func(r *http.Request) string { return "global" },
			Throttler:    throttling.NewThrottler(globalCfg, redisClient),
		},
		&throttling.ThrottleLevel{
			Name:         "per-ip",
			KeyExtractor: func(r *http.Request) string { return r.RemoteAddr },
			Throttler:    throttling.NewThrottler(ipCfg, redisClient),
		},
	)

	go ht.Start(context.Background())

	// Create load balancer
	lb := balancer.NewLoadBalancer(
		backendURLs,
		weights,
		cfg.HealthCheckInterval,
		cfg.MaxFailCount,
		strategy,
	)
	lb.Metrics = metrics

	mux := http.NewServeMux()

	mux.Handle("/", lb)
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/admin/reload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Reload configuration
		if err := reloadConfiguration(lb, *configPath); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Configuration reloaded successfully"))
	})

	// Start server
	server := http.Server{
		Addr:    cfg.ListenAddr,
		Handler: chain(mux, middleware.HierarchicalThrottlingMiddleware(ht)),
	}

	log.Printf("Starting load balancer on %s with strategy: %s", cfg.ListenAddr, cfg.Strategy)
	log.Printf("Metrics available at %s/metrics", cfg.ListenAddr)
	log.Fatal(server.ListenAndServe())
}

func reloadConfiguration(lb *balancer.LoadBalancer, configPath string) error {
	if configPath == "" {
		return fmt.Errorf("no config file provided")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}

	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("error parsing config file: %v", err)
	}

	strategy, err := config.ParseStrategyString(cfg.Strategy)
	if err != nil {
		return fmt.Errorf("invalid strategy: %v", err)
	}

	backendURLs := make([]string, len(cfg.Backends))
	weights := make([]int, len(cfg.Backends))

	// Extract backends and weights
	for i, backend := range cfg.Backends {
		backendURLs[i] = backend.URL
		weights[i] = backend.Weight
	}

	// Update load balancer configuration
	lb.Mux.Lock()
	lb.HealthCheckInterval = cfg.HealthCheckInterval
	lb.MaxFailCount = cfg.MaxFailCount
	lb.Strategy = strategy

	// Update backends (keep the existing ones if they're still in the config)
	oldBackends := lb.Backends
	lb.Backends = make([]*balancer.Backend, len(cfg.Backends))

	for i, backendURL := range backendURLs {
		// Check if this backend already exists
		found := false
		for _, oldBackend := range oldBackends {
			// Keep the existing backend but update its weight
			lb.Backends[i] = oldBackend
			oldBackend.Weight = weights[i]
			found = true
			break
		}

		if !found {
			parsedURL, _ := url.Parse(backendURL) // Error already checked earlier
			lb.Backends[i] = &balancer.Backend{
				URL:          parsedURL,
				Alive:        true, // Assume alive until health check
				ReverseProxy: balancer.CreateOptimizedReverseProxy(parsedURL),
				Weight:       weights[i],
			}
		}
	}

	lb.Mux.Unlock()

	log.Printf("Configuration reloaded with %d backends and strategy: %s", len(lb.Backends), cfg.Strategy)
	return nil
}

// Utility function to join paths
func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

func chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
