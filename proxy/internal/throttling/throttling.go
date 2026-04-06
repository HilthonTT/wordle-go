package throttling

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const RedisPrefix = "wordle-go-proxy"

type ThrottleConfig struct {
	MaxRequests int
	Interval    time.Duration
	Spans       int
	Cooldown    time.Duration
}

func NewDefaultThrottleConfig() ThrottleConfig {
	return ThrottleConfig{
		MaxRequests: 100,
		Interval:    time.Minute,
		Spans:       6,
		Cooldown:    30 * time.Second,
	}
}

type Throttler struct {
	config       ThrottleConfig
	client       *redis.Client
	spanDuration time.Duration

	mu            sync.RWMutex
	localCounts   map[string]int
	blockedRoutes map[string]time.Time

	// Adaptive node estimation
	nodesMu           sync.RWMutex
	nodes             float64
	previousSpanTotal int
	currentSpanTotal  int
}

func NewThrottler(config ThrottleConfig, client *redis.Client) *Throttler {
	return &Throttler{
		config:        config,
		client:        client,
		spanDuration:  config.Interval / time.Duration(config.Spans),
		localCounts:   make(map[string]int),
		blockedRoutes: make(map[string]time.Time),
		nodes:         1, // assume single node until we learn otherwise
	}
}

// Start aligns to span boundaries then ticks every spanDuration.
func (t *Throttler) Start(ctx context.Context) error {
	now := time.Now()
	spanSecs := int64(t.spanDuration.Seconds())
	nextSpanUnix := (now.Unix()/spanSecs + 1) * spanSecs
	initialDelay := time.Until(time.Unix(nextSpanUnix, 0))

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(initialDelay):
		t.processSpan(ctx)
	}

	ticker := time.NewTicker(t.spanDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			t.processSpan(ctx)
		}
	}
}

func (t *Throttler) processSpan(ctx context.Context) {
	// Atomically swap out the current counts.
	t.mu.Lock()
	localCounts := t.localCounts
	t.localCounts = make(map[string]int)
	t.mu.Unlock()

	// Fix: use integer nanosecond division, not float64 Seconds().
	intervalNum := time.Now().UnixNano() / int64(t.config.Interval)

	t.cleanupBlockedRoutes()
	t.updateNodeEstimation(ctx, localCounts, intervalNum)

	// Use a pipeline to batch IncrBy + Expire — halves round-trips.
	pipe := t.client.Pipeline()
	type entry struct {
		route   string
		count   int
		incrCmd *redis.IntCmd
	}
	var entries []entry

	for route, count := range localCounts {
		if count == 0 {
			continue
		}
		key := fmt.Sprintf("%s:%s:%d", RedisPrefix, route, intervalNum)
		incrCmd := pipe.IncrBy(ctx, key, int64(count))
		pipe.Expire(ctx, key, t.config.Interval*2)
		entries = append(entries, entry{route, count, incrCmd})
	}

	_, pipeErr := pipe.Exec(ctx)

	for _, e := range entries {
		if pipeErr == nil {
			val, err := e.incrCmd.Result()
			if err == nil && val > int64(t.config.MaxRequests) {
				t.blockRoute(e.route)
			}
		} else {
			// Redis unavailable: fall back to local threshold.
			localThreshold := t.config.MaxRequests / t.config.Spans
			if e.count > localThreshold {
				t.blockRoute(e.route)
			}
		}
	}
}

// updateNodeEstimation compares local traffic to the global Redis total
// from the previous interval to infer how many nodes are active.
func (t *Throttler) updateNodeEstimation(ctx context.Context, localCounts map[string]int, intervalNum int64) {
	var spanTotal int
	for _, c := range localCounts {
		spanTotal += c
	}

	t.nodesMu.Lock()
	t.currentSpanTotal += spanTotal
	t.nodesMu.Unlock()

	// Only recalculate at the last span of each interval.
	spanInInterval := (time.Now().UnixNano() / int64(t.spanDuration)) % int64(t.config.Spans)
	if spanInInterval != int64(t.config.Spans-1) {
		return
	}

	prevIntervalNum := intervalNum - 1
	var globalTotal int64
	for route := range localCounts {
		key := fmt.Sprintf("%s:%s:%d", RedisPrefix, route, prevIntervalNum)
		if val, err := t.client.Get(ctx, key).Int64(); err == nil {
			globalTotal += val
		}
	}

	t.nodesMu.Lock()
	defer t.nodesMu.Unlock()
	if globalTotal > 0 && t.previousSpanTotal > 0 {
		t.nodes = float64(globalTotal) / float64(t.previousSpanTotal)
	}
	t.previousSpanTotal = t.currentSpanTotal
	t.currentSpanTotal = 0
}

// IncrementAndCheck increments by weight (use 1 for normal requests).
// Returns false if the route is currently throttled.
func (t *Throttler) IncrementAndCheck(route string, weight int) bool {
	t.mu.RLock()
	if expiry, blocked := t.blockedRoutes[route]; blocked && time.Now().Before(expiry) {
		t.mu.RUnlock()
		return false
	}
	t.mu.RUnlock()

	// Early local check using node-estimated threshold.
	t.nodesMu.RLock()
	nodes := math.Max(1, t.nodes)
	t.nodesMu.RUnlock()

	localThreshold := int(float64(t.config.MaxRequests) / nodes)

	t.mu.Lock()
	t.localCounts[route] += weight
	localCount := t.localCounts[route]
	t.mu.Unlock()

	if localCount > localThreshold {
		t.blockRoute(route)
		return false
	}

	return true
}

func (t *Throttler) blockRoute(route string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.blockedRoutes[route] = time.Now().Add(t.config.Cooldown)
}

func (t *Throttler) cleanupBlockedRoutes() {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	for route, expiry := range t.blockedRoutes {
		if now.After(expiry) {
			delete(t.blockedRoutes, route)
		}
	}
}
