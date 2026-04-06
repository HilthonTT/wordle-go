package balancer

import (
	"net/http/httputil"
	"net/url"
	"sync"
)

type Backend struct {
	URL          *url.URL
	Alive        bool
	ReverseProxy *httputil.ReverseProxy
	Weight       int
	connections  int
	mux          sync.RWMutex
	failCount    int
}

// proxyBufferPool adapts sync.Pool to httputil.BufferPool.
type proxyBufferPool struct {
	pool sync.Pool
}

func newProxyBufferPool(size int) *proxyBufferPool {
	return &proxyBufferPool{
		pool: sync.Pool{
			New: func() any { return make([]byte, size) },
		},
	}
}

func (p *proxyBufferPool) Get() []byte {
	return p.pool.Get().([]byte)
}
func (p *proxyBufferPool) Put(b []byte) {
	p.pool.Put(b)
}

func (b *Backend) SetAlive(alive bool) {
	b.mux.Lock()
	b.Alive = alive
	if alive {
		b.failCount = 0
	}
	b.mux.Unlock()
}

func (b *Backend) IsAlive() bool {
	b.mux.RLock()
	alive := b.Alive
	b.mux.RUnlock()
	return alive
}

func (b *Backend) IncreaseFailCount() int {
	b.mux.Lock()
	b.failCount++
	count := b.failCount
	b.mux.Unlock()
	return count
}

func (b *Backend) ResetFailCount() {
	b.mux.Lock()
	b.failCount = 0
	b.mux.Unlock()
}
