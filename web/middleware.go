package web

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/funxdata/vango/errs"
)

// WithRequestID 把请求 ID 写入 request 的 context。
func WithRequestID(r *http.Request, id string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), requestIDKey, id))
}

// maxRequestIDLength 是请求 ID 的最大长度。
const maxRequestIDLength = 128

// validRequestID 只接受可见 ASCII，避免把不可信内容带进日志与响应头。
func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLength {
		return false
	}
	for i := 0; i < len(id); i++ {
		if id[i] < 0x21 || id[i] > 0x7e {
			return false
		}
	}
	return true
}

// RequestID 确保每个请求都有 ID：优先复用请求头中的值，否则生成一个。
// 生成结果会写入响应头，并可通过 Context.RequestID 读取。
func RequestID(generators ...func() string) Middleware {
	generate := randomRequestID
	if len(generators) > 0 && generators[0] != nil {
		generate = generators[0]
	}

	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			id := c.r.Header.Get(HeaderRequestID)
			if !validRequestID(id) {
				id = generate()
			}
			c.r = WithRequestID(c.r, id)
			c.SetHeader(HeaderRequestID, id)
			return next(c)
		}
	}
}

func randomRequestID() string {
	buf := make([]byte, 16)
	if _, err := crand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(buf)
}

// Recover 捕获处理过程中的 panic，记录堆栈并返回 500。
// logger 为 nil 时不记录日志。
func Recover(logger *slog.Logger) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) (err error) {
			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}
				if logger != nil {
					logger.Error("panic recovered",
						"error", fmt.Sprint(recovered),
						"method", c.r.Method,
						"path", c.r.URL.Path,
						"request_id", c.RequestID(),
						"stack", string(debug.Stack()),
					)
				}
				err = errs.Internal("internal server error")
			}()
			return next(c)
		}
	}
}

// LoggerOptions 定制访问日志中间件。
type LoggerOptions struct {
	// Logger 为 nil 时不记录日志。
	Logger *slog.Logger
	// SkipPaths 中的路径不记录访问日志，例如容器探针 /healthz、/readyz。
	SkipPaths []string
}

// Logger 为每个请求输出一条结构化访问日志。
func Logger(logger *slog.Logger) Middleware {
	return LoggerWithOptions(LoggerOptions{Logger: logger})
}

// LoggerWithOptions 按给定选项记录访问日志。
func LoggerWithOptions(opts LoggerOptions) Middleware {
	logger := opts.Logger
	if logger == nil {
		return func(next HandlerFunc) HandlerFunc { return next }
	}

	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			if contains(opts.SkipPaths, c.r.URL.Path) {
				return next(c)
			}

			start := time.Now()
			err := next(c)

			// 错误由外层 dispatch 统一写入响应，这里按 error 推算最终状态码，
			// 避免访问日志把失败的请求记成 200。
			status := c.Status()
			if err != nil && !c.Written() {
				if mapped := errs.From(err); mapped != nil {
					status = mapped.Status
				}
			}

			attrs := []any{
				"method", c.r.Method,
				"path", c.r.URL.Path,
				"status", status,
				"duration_ms", float64(time.Since(start).Microseconds()) / 1000,
				"ip", c.IP(),
				"request_id", c.RequestID(),
			}
			if err != nil {
				attrs = append(attrs, "error", err.Error())
				logger.Error("request failed", attrs...)
			} else {
				logger.Info("request", attrs...)
			}
			return err
		}
	}
}

// CORSOptions 描述跨域策略，零值表示允许任意来源且不允许携带凭证。
type CORSOptions struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           time.Duration
}

// SecureHeaders 为每个响应添加基础安全响应头。
//
// vango 只提供 JSON API，因此默认策略比较严格：
// 禁止内容嗅探、禁止被嵌入框架、不泄露 Referer。
func SecureHeaders() Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			header := c.w.Header()
			header.Set("X-Content-Type-Options", "nosniff")
			header.Set("X-Frame-Options", "DENY")
			header.Set("Referrer-Policy", "no-referrer")
			return next(c)
		}
	}
}

// CORS 按给定策略处理跨域请求，未传参数时使用宽松的默认策略。
func CORS(options ...CORSOptions) Middleware {
	opts := CORSOptions{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{
			http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
			http.MethodDelete, http.MethodHead, http.MethodOptions,
		},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", HeaderRequestID},
		MaxAge:       12 * time.Hour,
	}
	if len(options) > 0 {
		opts = options[0]
		if len(opts.AllowOrigins) == 0 {
			opts.AllowOrigins = []string{"*"}
		}
		if len(opts.AllowMethods) == 0 {
			opts.AllowMethods = []string{
				http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
				http.MethodDelete, http.MethodHead, http.MethodOptions,
			}
		}
		if opts.MaxAge == 0 {
			opts.MaxAge = 12 * time.Hour
		}
	}

	allowAll := contains(opts.AllowOrigins, "*")
	methods := strings.Join(opts.AllowMethods, ", ")
	headers := strings.Join(opts.AllowHeaders, ", ")
	exposed := strings.Join(opts.ExposeHeaders, ", ")
	maxAge := strconv.Itoa(int(opts.MaxAge.Seconds()))

	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			origin := c.r.Header.Get("Origin")
			if origin == "" {
				return next(c)
			}

			c.SetHeader("Vary", "Origin")
			switch {
			case allowAll && !opts.AllowCredentials:
				c.SetHeader("Access-Control-Allow-Origin", "*")
			case !allowAll && contains(opts.AllowOrigins, origin):
				c.SetHeader("Access-Control-Allow-Origin", origin)
			default:
				// 包括 AllowOrigins=["*"] + AllowCredentials=true 这一无效组合：
				// 通配来源不能与凭证一起使用，因此不回显 Origin。
				return next(c)
			}

			if opts.AllowCredentials {
				c.SetHeader("Access-Control-Allow-Credentials", "true")
			}
			if exposed != "" {
				c.SetHeader("Access-Control-Expose-Headers", exposed)
			}

			if c.r.Method == http.MethodOptions && c.r.Header.Get("Access-Control-Request-Method") != "" {
				c.SetHeader("Access-Control-Allow-Methods", methods)
				if headers != "" {
					c.SetHeader("Access-Control-Allow-Headers", headers)
				}
				c.SetHeader("Access-Control-Max-Age", maxAge)
				return c.NoContent()
			}

			return next(c)
		}
	}
}

// Timeout 给请求 context 加上超时。
//
// 它不会中断同步处理函数，但下游的数据库、HTTP 调用会因 context 到期而及时释放。
func Timeout(d time.Duration) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			ctx, cancel := context.WithTimeout(c.r.Context(), d)
			defer cancel()
			c.r = c.r.WithContext(ctx)

			err := next(c)
			if err == nil && ctx.Err() != nil {
				return errs.From(ctx.Err())
			}
			return err
		}
	}
}

// BodyLimit 限制请求体大小，超出时返回 413。
func BodyLimit(maxBytes int64) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			if c.r.ContentLength > maxBytes {
				return errs.New(http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE",
					fmt.Sprintf("request body exceeds %d bytes", maxBytes))
			}
			if c.r.Body != nil {
				c.r.Body = http.MaxBytesReader(c.w, c.r.Body, maxBytes)
			}
			return next(c)
		}
	}
}

// RateLimit 按客户端 IP 做令牌桶限流。
//
// rps 为每秒补充的令牌数，burst 为桶容量；超出时返回 429。
// 在反向代理之后部署时，先启用 RealIP，否则所有请求都会按代理地址共享同一个桶。
func RateLimit(rps float64, burst int) Middleware {
	if rps <= 0 {
		rps = 1
	}
	if burst <= 0 {
		burst = 1
	}

	var (
		mu      sync.Mutex
		buckets = make(map[string]*tokenBucket)
	)

	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			key := c.trustedIP()
			now := time.Now()

			mu.Lock()
			bucket, ok := buckets[key]
			if !ok {
				bucket = &tokenBucket{tokens: float64(burst), last: now}
				buckets[key] = bucket
			}
			bucket.tokens += now.Sub(bucket.last).Seconds() * rps
			if bucket.tokens > float64(burst) {
				bucket.tokens = float64(burst)
			}
			bucket.last = now

			allowed := bucket.tokens >= 1
			if allowed {
				bucket.tokens--
			}
			if len(buckets) > 10000 {
				for k, v := range buckets {
					if now.Sub(v.last) > time.Minute {
						delete(buckets, k)
					}
				}
			}
			mu.Unlock()

			if !allowed {
				c.SetHeader("Retry-After", "1")
				return errs.TooManyRequests("too many requests")
			}
			return next(c)
		}
	}
}

type tokenBucket struct {
	tokens float64
	last   time.Time
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
