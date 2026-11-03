package auth

import (
	"strings"

	"github.com/funxdata/vango/errs"
	"github.com/funxdata/vango/web"
)

// claimsStoreKey 是 Claims 在 web.Context 中的存储键。
const claimsStoreKey = "vango.auth.claims"

// MiddlewareOptions 定制鉴权中间件。
type MiddlewareOptions struct {
	// Header 是携带令牌的请求头，默认 Authorization。
	Header string
	// SkipPaths 中的路径会被跳过，例如登录与健康检查。
	SkipPaths []string
}

// MiddlewareOption 定制 MiddlewareOptions。
type MiddlewareOption func(*MiddlewareOptions)

// WithTokenHeader 指定携带令牌的请求头。
func WithTokenHeader(header string) MiddlewareOption {
	return func(o *MiddlewareOptions) { o.Header = header }
}

// WithSkipPaths 指定无需鉴权的路径。
func WithSkipPaths(paths ...string) MiddlewareOption {
	return func(o *MiddlewareOptions) { o.SkipPaths = append(o.SkipPaths, paths...) }
}

// Middleware 校验 Authorization: Bearer <token>，把 Claims 写入 Context。
//
//	api := r.Group("/api", auth.Middleware(signer, auth.WithSkipPaths("/api/login")))
//	api.GET("/me", func(c *web.Context) error {
//		claims, _ := auth.CurrentClaims(c)
//		return web.OK(c, claims)
//	})
func Middleware(signer *Signer, opts ...MiddlewareOption) web.Middleware {
	options := MiddlewareOptions{Header: "Authorization"}
	for _, opt := range opts {
		opt(&options)
	}

	return func(next web.HandlerFunc) web.HandlerFunc {
		return func(c *web.Context) error {
			for _, path := range options.SkipPaths {
				if path == c.Request().URL.Path {
					return next(c)
				}
			}

			token := BearerToken(c.Header(options.Header))
			if token == "" {
				return ErrMissingToken
			}

			claims, err := signer.Parse(token)
			if err != nil {
				return err
			}

			c.Set(claimsStoreKey, claims)
			return next(c)
		}
	}
}

// BearerToken 从请求头值中提取 Bearer 令牌，大小写不敏感。
func BearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}

	const prefix = "bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// CurrentClaims 返回当前请求的 Claims。
func CurrentClaims(c *web.Context) (*Claims, bool) {
	value, ok := c.Get(claimsStoreKey)
	if !ok {
		return nil, false
	}
	claims, ok := value.(*Claims)
	return claims, ok
}

// RequireRoles 要求当前请求的 Claims 至少具备其中一个角色。
func RequireRoles(roles ...string) web.Middleware {
	return func(next web.HandlerFunc) web.HandlerFunc {
		return func(c *web.Context) error {
			claims, ok := CurrentClaims(c)
			if !ok {
				return ErrMissingToken
			}
			for _, required := range roles {
				for _, role := range claims.Roles {
					if role == required {
						return next(c)
					}
				}
			}
			return errs.Forbidden("insufficient permissions")
		}
	}
}
