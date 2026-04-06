package router

import (
	"net/http"
)

// Group represents a group of routes
type Group struct {
	prefix     string
	middleware []Middleware
	router     *Router
}

// Group creates a new route group
func (r *Router) Group(prefix string) *Group {
	return &Group{
		prefix:     prefix,
		middleware: []Middleware{},
		router:     r,
	}
}

// Use adds middleware to the group
func (g *Group) Use(middleware ...Middleware) {
	g.middleware = append(g.middleware, middleware...)
}

// GET registers a handler for GET requests in this group
func (g *Group) GET(path string, handler http.HandlerFunc) {
	g.handle(http.MethodGet, path, handler)
}

// POST registers a handler for POST requests in this group
func (g *Group) POST(path string, handler http.HandlerFunc) {
	g.handle(http.MethodPost, path, handler)
}

// PUT registers a handler for PUT requests in this group
func (g *Group) PUT(path string, handler http.HandlerFunc) {
	g.handle(http.MethodPut, path, handler)
}

// DELETE registers a handler for DELETE requests in this group
func (g *Group) DELETE(path string, handler http.HandlerFunc) {
	g.handle(http.MethodDelete, path, handler)
}

// handle registers a handler with group prefix and middleware
func (g *Group) handle(method, path string, handler http.HandlerFunc) {
	// Combine group middleware with the handler
	var h http.Handler = http.HandlerFunc(handler)
	for i := len(g.middleware) - 1; i >= 0; i-- {
		h = g.middleware[i](h)
	}

	// Register the route with the main router
	fullPath := g.prefix + path
	g.router.Handle(method, fullPath, h.(http.HandlerFunc))

	logPath(method, path)
}
