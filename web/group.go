package web

// Group 是带统一前缀与中间件的路由分组。
type Group struct {
	router *Router
	prefix string
	mws    []Middleware
}

// Group 创建一个路由分组。
//
//	api := r.Group("/api/v1", auth.Middleware(verifier))
//	api.GET("/users/:id", handler)
func (r *Router) Group(prefix string, mws ...Middleware) *Group {
	return &Group{router: r, prefix: joinPath("", prefix), mws: mws}
}

// Use 追加分组中间件，对之后注册的路由生效。
func (g *Group) Use(mw ...Middleware) {
	g.mws = append(g.mws, mw...)
}

// Group 在分组内再创建子分组，继承父分组的中间件。
func (g *Group) Group(prefix string, mws ...Middleware) *Group {
	combined := make([]Middleware, 0, len(g.mws)+len(mws))
	combined = append(combined, g.mws...)
	combined = append(combined, mws...)
	return &Group{router: g.router, prefix: joinPath(g.prefix, prefix), mws: combined}
}

// Handle 在分组内注册路由。
func (g *Group) Handle(method, path string, h HandlerFunc) {
	g.router.Handle(method, joinPath(g.prefix, path), apply(h, g.mws))
}

// GET 在分组内注册 GET 路由。
func (g *Group) GET(path string, h HandlerFunc) { g.Handle("GET", path, h) }

// POST 在分组内注册 POST 路由。
func (g *Group) POST(path string, h HandlerFunc) { g.Handle("POST", path, h) }

// PUT 在分组内注册 PUT 路由。
func (g *Group) PUT(path string, h HandlerFunc) { g.Handle("PUT", path, h) }

// PATCH 在分组内注册 PATCH 路由。
func (g *Group) PATCH(path string, h HandlerFunc) { g.Handle("PATCH", path, h) }

// DELETE 在分组内注册 DELETE 路由。
func (g *Group) DELETE(path string, h HandlerFunc) { g.Handle("DELETE", path, h) }

// HEAD 在分组内注册 HEAD 路由。
func (g *Group) HEAD(path string, h HandlerFunc) { g.Handle("HEAD", path, h) }

// OPTIONS 在分组内注册 OPTIONS 路由。
func (g *Group) OPTIONS(path string, h HandlerFunc) { g.Handle("OPTIONS", path, h) }

// Any 在分组内为所有常见方法注册同一处理函数。
func (g *Group) Any(path string, h HandlerFunc) {
	for _, method := range Methods {
		g.Handle(method, path, h)
	}
}
