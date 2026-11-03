package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/funxdata/vango/errs"
)

// contextKey 是本包私有的 context 键类型，避免与业务键冲突。
type contextKey int

const (
	requestIDKey contextKey = iota
)

// HeaderRequestID 是请求 ID 的头部名称。
const HeaderRequestID = "X-Request-Id"

// Context 封装一次 API 请求的读写，是业务处理函数的唯一入参。
//
// vango 只处理 JSON 与查询参数，不提供 HTML 模板、静态文件或 Cookie 会话能力。
type Context struct {
	w       http.ResponseWriter
	r       *http.Request
	params  Params
	store   map[string]any
	err     error
	status  int
	written bool
}

func newContext(w http.ResponseWriter, r *http.Request) *Context {
	return &Context{w: w, r: r}
}

// Request 返回原始 *http.Request。
func (c *Context) Request() *http.Request { return c.r }

// ResponseWriter 返回底层 http.ResponseWriter。
func (c *Context) ResponseWriter() http.ResponseWriter { return c.w }

// StdContext 返回请求的 context.Context，用于下游数据库与 HTTP 调用。
func (c *Context) StdContext() context.Context { return c.r.Context() }

// Param 返回路径参数。
func (c *Context) Param(name string) string { return c.params.Get(name) }

// ParamInt 返回整数路径参数，无法解析时返回 def。
func (c *Context) ParamInt(name string, def int) int {
	n, err := strconv.Atoi(c.Param(name))
	if err != nil {
		return def
	}
	return n
}

// Params 返回全部路径参数。
func (c *Context) Params() Params { return c.params }

// Query 返回查询参数。
func (c *Context) Query(name string) string { return c.r.URL.Query().Get(name) }

// QueryDefault 返回查询参数，缺失时使用 def。
func (c *Context) QueryDefault(name, def string) string {
	if v := c.r.URL.Query().Get(name); v != "" {
		return v
	}
	return def
}

// QueryInt 返回整数查询参数，无法解析时返回 def。
func (c *Context) QueryInt(name string, def int) int {
	n, err := strconv.Atoi(c.r.URL.Query().Get(name))
	if err != nil {
		return def
	}
	return n
}

// QueryBool 返回布尔查询参数，无法解析时返回 def。
func (c *Context) QueryBool(name string, def bool) bool {
	b, err := strconv.ParseBool(c.r.URL.Query().Get(name))
	if err != nil {
		return def
	}
	return b
}

// QueryStrings 返回可重复的查询参数，例如 ?tag=a&tag=b。
func (c *Context) QueryStrings(name string) []string { return c.r.URL.Query()[name] }

// Header 返回请求头。
func (c *Context) Header(name string) string { return c.r.Header.Get(name) }

// SetHeader 设置响应头。
func (c *Context) SetHeader(name, value string) { c.w.Header().Set(name, value) }

// ContentType 返回请求的 Content-Type。
func (c *Context) ContentType() string { return c.r.Header.Get("Content-Type") }

// IP 返回客户端 IP，依次尝试 X-Forwarded-For、X-Real-IP 与 RemoteAddr。
//
// X-Forwarded-For / X-Real-IP 可以被客户端伪造，未启用 RealIP 中间件时
// 只适合日志与限流等非安全用途；部署在反向代理之后时，请先启用
// web.RealIP(web.TrustedProxies()...) 再依赖 IP() 做鉴权决策。
func (c *Context) IP() string {
	if ip, ok := c.store[clientIPKey].(string); ok && ip != "" {
		return ip
	}

	if forwarded := c.r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if idx := strings.IndexByte(forwarded, ','); idx >= 0 {
			forwarded = forwarded[:idx]
		}
		if ip := strings.TrimSpace(forwarded); ip != "" {
			return ip
		}
	}
	if realIP := strings.TrimSpace(c.r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	return remoteIP(c.r.RemoteAddr)
}

// ClientIP 返回经过可信代理校验的客户端 IP。
//
// 只有直连对端位于 trustedProxies 中时才会采信 X-Forwarded-For / X-Real-IP，
// 否则直接使用 RemoteAddr，避免伪造请求头绕过限流或污染审计日志。
func (c *Context) ClientIP(trustedProxies ...*net.IPNet) string {
	return resolveClientIP(c.r, trustedProxies)
}

// RequestID 返回本次请求的 ID。
func (c *Context) RequestID() string {
	if id, ok := c.r.Context().Value(requestIDKey).(string); ok {
		return id
	}
	return c.r.Header.Get(HeaderRequestID)
}

// Status 返回已写入的响应状态码，尚未写入时返回 200。
func (c *Context) Status() int {
	if c.status == 0 {
		return http.StatusOK
	}
	return c.status
}

// Written 表示响应是否已开始写入。
func (c *Context) Written() bool { return c.written }

// Err 返回处理函数产生的错误，供中间件记录日志。
func (c *Context) Err() error { return c.err }

// Set 在请求范围内保存一个值。
func (c *Context) Set(key string, value any) {
	if c.store == nil {
		c.store = make(map[string]any)
	}
	c.store[key] = value
}

// Get 读取请求范围内保存的值。
func (c *Context) Get(key string) (any, bool) {
	v, ok := c.store[key]
	return v, ok
}

// GetString 读取字符串值，不存在或类型不符时返回 ""。
func (c *Context) GetString(key string) string {
	v, _ := c.store[key].(string)
	return v
}

// Body 读取请求体，服务端会自动关闭请求体。
func (c *Context) Body() ([]byte, error) {
	if c.r.Body == nil {
		return nil, nil
	}
	return io.ReadAll(c.r.Body)
}

// Bind 解析 JSON 请求体到 v。
func (c *Context) Bind(v any) error { return c.BindJSON(v) }

// BindJSON 解析 JSON 请求体到 v。
func (c *Context) BindJSON(v any) error {
	if c.r.Body == nil {
		return errs.BadRequest("request body is empty")
	}

	decoder := json.NewDecoder(c.r.Body)
	if err := decoder.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return errs.BadRequest("request body is empty")
		}
		return errs.BadRequest("invalid JSON body: " + err.Error())
	}
	return nil
}

// BindQuery 把查询参数（含 path 之外的 filter/sort/page 等）绑定到 v。
func (c *Context) BindQuery(v any) error {
	return bindQuery(v, c.r.URL.Query())
}

// Validator 由业务类型实现，用于在绑定后做业务校验。
//
//	type CreateUser struct {
//		Name string `json:"name"`
//	}
//
//	func (r *CreateUser) Validate() error {
//		if r.Name == "" { return errs.BadRequest("name is required") }
//		return nil
//	}
type Validator interface {
	Validate() error
}

// Validate 调用 v 实现的 Validate，未实现时直接返回 nil。
func (c *Context) Validate(v any) error {
	if validator, ok := v.(Validator); ok {
		return validator.Validate()
	}
	return nil
}

// BindAndValidate 先解析 JSON 再校验。
func (c *Context) BindAndValidate(v any) error {
	if err := c.BindJSON(v); err != nil {
		return err
	}
	return c.Validate(v)
}

// JSON 写入 JSON 响应。
func (c *Context) JSON(status int, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.SetHeader("Content-Type", "application/json; charset=utf-8")
	return c.write(status, body)
}

// NoContent 返回 204 响应，适合 DELETE / PUT 成功后的空响应。
func (c *Context) NoContent() error {
	if c.written {
		return nil
	}
	c.status = http.StatusNoContent
	c.written = true
	c.w.WriteHeader(http.StatusNoContent)
	return nil
}

// fail 把错误转换为统一的错误响应，并记录在 Context 上供中间件使用。
func (c *Context) fail(err error) {
	c.err = err
	if c.written {
		return
	}
	_ = Fail(c, err)
}

func (c *Context) write(status int, body []byte) error {
	if c.written {
		return nil
	}
	c.status = status
	c.written = true
	c.w.WriteHeader(status)
	if len(body) == 0 {
		return nil
	}
	_, err := c.w.Write(body)
	return err
}
