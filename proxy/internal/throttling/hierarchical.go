package throttling

import (
	"context"
	"net/http"
)

type ThrottleLevel struct {
	Name         string
	KeyExtractor func(r *http.Request) string
	Throttler    *Throttler
	Weight       int // request cost; 1 for most routes
}

type HierarchicalThrottler struct {
	levels []*ThrottleLevel
}

func NewHierarchicalThrottler(levels ...*ThrottleLevel) *HierarchicalThrottler {
	return &HierarchicalThrottler{levels: levels}
}

// Start launches all throttler background goroutines.
func (h *HierarchicalThrottler) Start(ctx context.Context) error {
	errc := make(chan error, len(h.levels))
	for _, level := range h.levels {
		go func(l *ThrottleLevel) {
			errc <- l.Throttler.Start(ctx)
		}(level)
	}
	// Return first error (including ctx cancellation).
	return <-errc
}

// CheckRequest returns false as soon as any level denies the request.
func (h *HierarchicalThrottler) CheckRequest(r *http.Request) bool {
	for _, level := range h.levels {
		key := level.KeyExtractor(r)
		weight := level.Weight
		if weight == 0 {
			weight = 1
		}
		if !level.Throttler.IncrementAndCheck(key, weight) {
			return false
		}
	}
	return true
}
