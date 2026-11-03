// Package web 是 vango 的 HTTP 层：路由、请求上下文、中间件与统一响应。
//
// 快速上手：
//
//	r := web.NewRouter()
//	r.Use(web.Recover(logger), web.RequestID(), web.Logger(logger))
//	r.GET("/hello/:name", func(c *web.Context) error {
//		return web.OK(c, map[string]string{"hello": c.Param("name")})
//	})
//	web.NewServer(":8080", r).Start()
package web

import (
	"net/http"
	"strings"
	"sync"

	"github.com/funxdata/vango/errs"
)

// HandlerFunc 是 vango 的请求处理函数。
//
// 返回 error 时框架会把它转换为 JSON 响应：*errs.Error 按其状态码输出，
// 其余错误统一返回 500，原始错误保留在日志中。
type HandlerFunc func(*Context) error

// Middleware 包裹一个 HandlerFunc，多个中间件按声明顺序由外向内执行。
type Middleware func(HandlerFunc) HandlerFunc

// Route 描述一条已注册的路由。
type Route struct {
	Method  string
	Pattern string
}

// node 是路由树的节点，每个节点对应 URL 中的一个路径段。
type node struct {
	static       map[string]*node
	param        *node
	paramName    string
	wildcard     *node
	wildcardName string
	handlers     map[string]HandlerFunc
}

// Router 是基于路径段树的 HTTP 路由，支持 :param 与 *wildcard 两种动态段。
//
// 路由注册应在服务启动前完成；启动后并发读写路由表需要调用方自行保证。
type Router struct {
	mu               sync.RWMutex
	root             *node
	middlewares      []Middleware
	notFound         HandlerFunc
	methodNotAllowed HandlerFunc
	routes           []Route
}

// NewRouter 创建一个空路由。
func NewRouter() *Router {
	return &Router{
		root: &node{},
		notFound: func(c *Context) error {
			return errs.New(http.StatusNotFound, "ROUTE_NOT_FOUND",
				"route not found: "+c.Request().Method+" "+c.Request().URL.Path)
		},
		methodNotAllowed: func(c *Context) error {
			return errs.New(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED",
				"method not allowed: "+c.Request().Method)
		},
	}
}

// Use 注册全局中间件，必须在启动服务前调用。
func (r *Router) Use(mw ...Middleware) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.middlewares = append(r.middlewares, mw...)
}

// Handle 注册一条路由。
//
// pattern 必须以 / 开头，动态段写作 :name，末尾的通配段写作 *name。
func (r *Router) Handle(method, pattern string, h HandlerFunc) {
	if h == nil {
		panic("web: nil handler for " + method + " " + pattern)
	}
	if !strings.HasPrefix(pattern, "/") {
		panic("web: pattern must start with '/': " + pattern)
	}

	method = strings.ToUpper(method)
	segments := splitPath(pattern)

	r.mu.Lock()
	defer r.mu.Unlock()

	n := r.root
	for i, segment := range segments {
		switch {
		case strings.HasPrefix(segment, "*"):
			if i != len(segments)-1 {
				panic("web: wildcard must be the last segment: " + pattern)
			}
			if n.wildcard == nil {
				n.wildcard = &node{}
			}
			n.wildcard.wildcardName = segment[1:]
			n = n.wildcard
		case strings.HasPrefix(segment, ":"):
			if n.param == nil {
				n.param = &node{}
			}
			n.param.paramName = segment[1:]
			n = n.param
		default:
			if n.static == nil {
				n.static = make(map[string]*node)
			}
			child, ok := n.static[segment]
			if !ok {
				child = &node{}
				n.static[segment] = child
			}
			n = child
		}
	}

	if n.handlers == nil {
		n.handlers = make(map[string]HandlerFunc)
	}
	n.handlers[method] = h
	r.routes = append(r.routes, Route{Method: method, Pattern: pattern})
}

// GET 注册 GET 路由。
func (r *Router) GET(pattern string, h HandlerFunc) { r.Handle(http.MethodGet, pattern, h) }

// POST 注册 POST 路由。
func (r *Router) POST(pattern string, h HandlerFunc) { r.Handle(http.MethodPost, pattern, h) }

// PUT 注册 PUT 路由。
func (r *Router) PUT(pattern string, h HandlerFunc) { r.Handle(http.MethodPut, pattern, h) }

// PATCH 注册 PATCH 路由。
func (r *Router) PATCH(pattern string, h HandlerFunc) { r.Handle(http.MethodPatch, pattern, h) }

// DELETE 注册 DELETE 路由。
func (r *Router) DELETE(pattern string, h HandlerFunc) { r.Handle(http.MethodDelete, pattern, h) }

// HEAD 注册 HEAD 路由。
func (r *Router) HEAD(pattern string, h HandlerFunc) { r.Handle(http.MethodHead, pattern, h) }

// OPTIONS 注册 OPTIONS 路由。
func (r *Router) OPTIONS(pattern string, h HandlerFunc) { r.Handle(http.MethodOptions, pattern, h) }

// Methods 列出 Any 会注册的全部方法。
var Methods = []string{
	http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
	http.MethodDelete, http.MethodHead, http.MethodOptions,
}

// Any 为所有常见方法注册同一处理函数。
func (r *Router) Any(pattern string, h HandlerFunc) {
	for _, method := range Methods {
		r.Handle(method, pattern, h)
	}
}

// Routes 返回已注册路由的快照。
func (r *Router) Routes() []Route {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Route, len(r.routes))
	copy(out, r.routes)
	return out
}

// ServeHTTP 实现 http.Handler。
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.dispatch(newContext(w, req))
}

func (r *Router) dispatch(c *Context) {
	r.mu.RLock()
	mws := make([]Middleware, len(r.middlewares))
	copy(mws, r.middlewares)
	notFound, methodNotAllowed := r.notFound, r.methodNotAllowed
	r.mu.RUnlock()

	path := c.Request().URL.Path
	handler, params, pathMatched := r.match(c.Request().Method, splitPath(path))
	if handler == nil && c.Request().Method == http.MethodHead {
		// HEAD 未单独注册时复用 GET。
		handler, params, pathMatched = r.match(http.MethodGet, splitPath(path))
	}

	c.params = params
	if handler == nil {
		if pathMatched {
			handler = methodNotAllowed
		} else {
			handler = notFound
		}
	}

	if err := apply(handler, mws)(c); err != nil {
		c.fail(err)
	}
}

func (r *Router) match(method string, segments []string) (HandlerFunc, Params, bool) {
	r.mu.RLock()
	root := r.root
	r.mu.RUnlock()

	return search(root, segments, nil, method)
}

func search(n *node, segments []string, params Params, method string) (HandlerFunc, Params, bool) {
	if n == nil {
		return nil, nil, false
	}

	if len(segments) == 0 {
		if h, ok := n.handlers[method]; ok {
			return h, params, true
		}
		if n.wildcard != nil {
			if h, ok := n.wildcard.handlers[method]; ok {
				return h, appendParam(params, n.wildcard.wildcardName, ""), true
			}
		}
		return nil, nil, len(n.handlers) > 0
	}

	segment := segments[0]

	if child, ok := n.static[segment]; ok {
		if h, p, matched := search(child, segments[1:], params, method); h != nil {
			return h, p, true
		} else if matched {
			return nil, nil, true
		}
	}

	if n.param != nil {
		if h, p, matched := search(n.param, segments[1:], appendParam(params, n.param.paramName, segment), method); h != nil {
			return h, p, true
		} else if matched {
			return nil, nil, true
		}
	}

	if n.wildcard != nil {
		if h, ok := n.wildcard.handlers[method]; ok {
			return h, appendParam(params, n.wildcard.wildcardName, strings.Join(segments, "/")), true
		}
	}

	return nil, nil, false
}

func apply(h HandlerFunc, mws []Middleware) HandlerFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// Chain 把多个中间件合并为一个，执行顺序为参数顺序由外向内。
func Chain(mws ...Middleware) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return apply(next, mws)
	}
}

func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func joinPath(prefix, path string) string {
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if prefix == "" || prefix == "/" {
		return path
	}
	return strings.TrimSuffix(prefix, "/") + path
}

// Params 保存路径参数，按注册顺序排列。
type Params []Param

// Param 是一个路径参数。
type Param struct {
	Key   string
	Value string
}

// Get 返回指定名称的路径参数。
func (p Params) Get(key string) string {
	for _, param := range p {
		if param.Key == key {
			return param.Value
		}
	}
	return ""
}

// Has 判断路径参数是否存在。
func (p Params) Has(key string) bool {
	for _, param := range p {
		if param.Key == key {
			return true
		}
	}
	return false
}

func appendParam(params Params, key, value string) Params {
	out := make(Params, len(params), len(params)+1)
	copy(out, params)
	return append(out, Param{Key: key, Value: value})
}
