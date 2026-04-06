package balancer

import (
	"fmt"
	"hash/fnv"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

type Strategy int

const (
	RoundRobin Strategy = iota
	LeastConnections
	IPHash
	Random
	WeightedRoundRobin
)

type LoadBalancer struct {
	Backends            []*Backend
	Current             int
	Mux                 sync.Mutex
	HealthCheckInterval time.Duration
	MaxFailCount        int
	Strategy            Strategy
	Metrics             *Metrics
	atomicCurrent       uint32 // atomic counter for lock-free round robin
}

// Shared buffer pool for all reverse proxies.
var sharedBufferPool = newProxyBufferPool(32 * 1024)

func NewLoadBalancer(
	backendURLs []string,
	weights []int,
	healthCheckInterval time.Duration,
	maxFailCount int,
	strategy Strategy,
) *LoadBalancer {
	if len(weights) == 0 {
		weights = make([]int, len(backendURLs))
		for i := range weights {
			weights[i] = 1
		}
	}

	backends := make([]*Backend, len(backendURLs))

	for i, rawURL := range backendURLs {
		parsedURL, err := url.Parse(rawURL)
		if err != nil {
			log.Fatal(err)
		}

		backends[i] = &Backend{
			URL:          parsedURL,
			Alive:        true,
			ReverseProxy: CreateOptimizedReverseProxy(parsedURL),
			Weight:       weights[i],
		}
	}

	lb := &LoadBalancer{
		Backends:            backends,
		HealthCheckInterval: healthCheckInterval,
		MaxFailCount:        maxFailCount,
		Strategy:            strategy,
	}

	for _, backend := range backends {
		b := backend
		b.ReverseProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			failCount := b.IncreaseFailCount()
			log.Printf("Backend %s request failed: %v, fail count: %d", b.URL.Host, err, failCount)

			if failCount >= lb.MaxFailCount {
				log.Printf("Backend %s is marked as down due to too many failures", b.URL.Host)
				b.SetAlive(false)
			}

			if next := lb.NextBackend(); next != nil {
				log.Printf("Retrying request on backend %s", next.URL.Host)
				next.ReverseProxy.ServeHTTP(w, r)
				return
			}

			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		}
	}

	go lb.healthCheck()

	return lb
}

func CreateOptimizedReverseProxy(target *url.URL) *httputil.ReverseProxy {
	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = singleJoiningSlash(target.Path, req.URL.Path)

		if req.Header.Get("Host") == "" {
			req.Host = target.Host
		}

		targetQuery := target.RawQuery
		if targetQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = targetQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
		}
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	return &httputil.ReverseProxy{
		Director:   director,
		Transport:  transport,
		BufferPool: sharedBufferPool,
	}
}

func (lb *LoadBalancer) chooseBackendByStrategy(r *http.Request) *Backend {
	lb.Mux.Lock()
	defer lb.Mux.Unlock()

	aliveCount := 0
	for _, b := range lb.Backends {
		if b.IsAlive() {
			aliveCount++
		}
	}

	if aliveCount == 0 {
		return nil
	}

	switch lb.Strategy {
	case RoundRobin:
		return lb.roundRobinSelect()
	case LeastConnections:
		return lb.leastConnectionsSelect()
	case IPHash:
		return lb.ipHashSelect(r)
	case Random:
		return lb.randomSelect()
	case WeightedRoundRobin:
		return lb.weightedRoundRobinSelect()
	default:
		return lb.roundRobinSelect()
	}
}

func (lb *LoadBalancer) roundRobinSelect() *Backend {
	numBackends := len(lb.Backends)

	for i := 0; i < numBackends; i++ {
		idx := int(atomic.AddUint32(&lb.atomicCurrent, 1)) % numBackends
		if lb.Backends[idx].IsAlive() {
			return lb.Backends[idx]
		}
	}

	return nil
}

func (lb *LoadBalancer) leastConnectionsSelect() *Backend {
	var leastConnBackend *Backend
	leastConn := -1

	for _, b := range lb.Backends {
		if !b.IsAlive() {
			continue
		}

		b.mux.RLock()
		connCount := b.connections
		b.mux.RUnlock()

		if leastConn == -1 || connCount < leastConn {
			leastConn = connCount
			leastConnBackend = b
		}
	}

	return leastConnBackend
}

func (lb *LoadBalancer) ipHashSelect(r *http.Request) *Backend {
	ip := getClientIP(r)

	hash := fnv.New32()
	hash.Write([]byte(ip))
	idx := hash.Sum32() % uint32(len(lb.Backends))

	initialIdx := idx
	for i := 0; i < len(lb.Backends); i++ {
		checkIdx := (initialIdx + uint32(i)) % uint32(len(lb.Backends))
		if lb.Backends[checkIdx].IsAlive() {
			return lb.Backends[checkIdx]
		}
	}

	return nil
}

func (lb *LoadBalancer) randomSelect() *Backend {
	var aliveIndices []int
	for i, b := range lb.Backends {
		if b.IsAlive() {
			aliveIndices = append(aliveIndices, i)
		}
	}

	if len(aliveIndices) == 0 {
		return nil
	}

	randomIdx := aliveIndices[rand.Intn(len(aliveIndices))]
	return lb.Backends[randomIdx]
}

func (lb *LoadBalancer) weightedRoundRobinSelect() *Backend {
	totalWeight := 0
	for _, b := range lb.Backends {
		if b.IsAlive() {
			totalWeight += b.Weight
		}
	}

	if totalWeight == 0 {
		return nil
	}

	targetWeight := rand.Intn(totalWeight)
	currentWeight := 0

	for _, b := range lb.Backends {
		if !b.IsAlive() {
			continue
		}

		currentWeight += b.Weight
		if targetWeight < currentWeight {
			return b
		}
	}

	return lb.roundRobinSelect()
}

func (lb *LoadBalancer) NextBackend() *Backend {
	lb.Mux.Lock()
	defer lb.Mux.Unlock()

	initialIndex := lb.Current

	for {
		lb.Current = (lb.Current + 1) % len(lb.Backends)
		if lb.Backends[lb.Current].IsAlive() {
			return lb.Backends[lb.Current]
		}

		if lb.Current == initialIndex {
			return nil
		}
	}
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	backend := lb.chooseBackendByStrategy(r)
	if backend == nil {
		http.Error(w, "No available backends", http.StatusServiceUnavailable)
		lb.Metrics.requestCount.WithLabelValues("none", "503", r.Method).Inc()
		return
	}

	start := time.Now()

	backend.mux.Lock()
	backend.connections++
	backend.mux.Unlock()

	backendLabel := backend.URL.Host
	lb.Metrics.activeConnections.WithLabelValues(backendLabel).Inc()

	wrappedWriter := &metricsResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	log.Printf("Forwarding request to: %s", backend.URL.Host)
	backend.ReverseProxy.ServeHTTP(wrappedWriter, r)

	duration := time.Since(start).Seconds()

	backend.mux.Lock()
	backend.connections--
	backend.mux.Unlock()

	lb.Metrics.activeConnections.WithLabelValues(backendLabel).Dec()

	statusCode := fmt.Sprintf("%d", wrappedWriter.statusCode)
	lb.Metrics.requestCount.WithLabelValues(backendLabel, statusCode, r.Method).Inc()
	lb.Metrics.requestDuration.WithLabelValues(backendLabel).Observe(duration)
	lb.Metrics.backendResponseTime.WithLabelValues(backendLabel).Observe(duration)

	if wrappedWriter.statusCode < 500 {
		backend.ResetFailCount()
	} else {
		lb.Metrics.backendErrors.WithLabelValues(backendLabel, "response_error").Inc()
	}
}

func (lb *LoadBalancer) healthCheck() {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   2 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   2 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   3 * time.Second,
	}

	ticker := time.NewTicker(lb.HealthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		results := make(chan struct {
			index int
			alive bool
		}, len(lb.Backends))

		for i, backend := range lb.Backends {
			go func(i int, backend *Backend) {
				alive := isBackendAliveHTTP(backend.URL, client)
				results <- struct {
					index int
					alive bool
				}{i, alive}
			}(i, backend)
		}

		for i := 0; i < len(lb.Backends); i++ {
			result := <-results
			backend := lb.Backends[result.index]
			backend.SetAlive(result.alive)

			backendLabel := backend.URL.Host
			if result.alive {
				lb.Metrics.backendUpGauge.WithLabelValues(backendLabel).Set(1)
			} else {
				lb.Metrics.backendUpGauge.WithLabelValues(backendLabel).Set(0)
				lb.Metrics.backendErrors.WithLabelValues(backendLabel, "health_check").Inc()
			}
		}
	}
}

func isBackendAliveHTTP(u *url.URL, client *http.Client) bool {
	resp, err := client.Get(u.String() + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < http.StatusInternalServerError
}
