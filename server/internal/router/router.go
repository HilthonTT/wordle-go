package router

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const paramsKey contextKey = "params"

// Router is our HTTP router.
type Router struct {
	root       *Node
	notFound   http.HandlerFunc
	middleware []Middleware
}

func New() *Router {
	return &Router{
		root: &Node{
			part:     "",
			children: []*Node{},
			handlers: make(map[string]http.HandlerFunc),
		},
		notFound: http.NotFound,
	}
}

func (r *Router) GET(path string, handler http.HandlerFunc) { r.Handle(http.MethodGet, path, handler) }
func (r *Router) POST(path string, handler http.HandlerFunc) {
	r.Handle(http.MethodPost, path, handler)
}
func (r *Router) PUT(path string, handler http.HandlerFunc) { r.Handle(http.MethodPut, path, handler) }
func (r *Router) PATCH(path string, handler http.HandlerFunc) {
	r.Handle(http.MethodPatch, path, handler)
}
func (r *Router) DELETE(path string, handler http.HandlerFunc) {
	r.Handle(http.MethodDelete, path, handler)
}

func (r *Router) NotFound(handler http.HandlerFunc) { r.notFound = handler }

func (r *Router) Use(middleware ...Middleware) {
	r.middleware = append(r.middleware, middleware...)
}

// Handle registers a new handler for the given method and path.
func (r *Router) Handle(method, path string, handler http.HandlerFunc) {
	if len(path) == 0 || path[0] != '/' {
		panic("path must begin with '/'")
	}

	segments := splitPath(path)

	// Truncate everything after a wildcard segment.
	for i, seg := range segments {
		if seg == "*" {
			segments = segments[:i+1]
			break
		}
	}

	r.root.insert(segments, method, handler, 0)

	logPath(method, path)
}

// ServeHTTP implements http.Handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	segments := splitPath(req.URL.Path)
	params := make(map[string]string)

	handler := r.findHandler(segments, r.root, req.Method, params)
	if handler == nil {
		if r.notFound != nil {
			handler = r.notFound
		} else {
			handler = http.NotFound
		}
	}

	if len(params) > 0 {
		ctx := context.WithValue(req.Context(), paramsKey, params)
		req = req.WithContext(ctx)
	}

	// Apply middleware in reverse order so the first-registered runs outermost.
	var h http.Handler = http.HandlerFunc(handler)
	for i := len(r.middleware) - 1; i >= 0; i-- {
		h = r.middleware[i](h)
	}

	h.ServeHTTP(w, req)
}

// findHandler recursively resolves a handler for the given path segments.
//
// BUG FIX: the original implementation checked wildcards inside the first loop
// (before param matching), meaning a wildcard could shadow a more-specific
// static or param route registered later. Priority must be:
//
//  1. Exact static match  →  2. Named param  →  3. Wildcard (last resort)
func (r *Router) findHandler(segments []string, node *Node, method string, params map[string]string) http.HandlerFunc {
	if len(segments) == 0 {
		if h, ok := node.handlers[method]; ok {
			return h
		}
		return nil
	}

	segment := segments[0]
	remaining := segments[1:]

	// 1. Static children (sorted alphabetically, fastest path).
	for _, child := range node.children {
		if child.isParam || child.isWildcard {
			break // children are ordered: static → param → wildcard
		}
		if child.part == segment {
			if h := r.findHandler(remaining, child, method, params); h != nil {
				return h
			}
		}
	}

	// 2. Named-param children.
	for _, child := range node.children {
		if !child.isParam {
			continue
		}
		params[child.part] = segment
		if h := r.findHandler(remaining, child, method, params); h != nil {
			return h
		}
		delete(params, child.part)
	}

	// 3. Wildcard children — consumes all remaining segments.
	for _, child := range node.children {
		if !child.isWildcard {
			continue
		}
		if h, ok := child.handlers[method]; ok {
			params["*"] = strings.Join(segments, "/")
			return h
		}
	}

	return nil
}

// Params returns the URL parameters stored in the request context.
func Params(r *http.Request) map[string]string {
	params, _ := r.Context().Value(paramsKey).(map[string]string)
	return params
}

func splitPath(path string) []string {
	parts := strings.Split(path, "/")
	result := parts[:0]
	for _, s := range parts {
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}
