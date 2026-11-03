package web

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// clientIPKey 是 RealIP 中间件在 Context 中保存客户端 IP 的键。
const clientIPKey = "vango.web.client_ip"

// ParseTrustedProxies 把 IP 或 CIDR 字符串解析为可信代理网段。
//
//	proxies, err := web.ParseTrustedProxies("10.0.0.0/8", "172.17.0.1")
func ParseTrustedProxies(cidrs ...string) ([]*net.IPNet, error) {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}

		if !strings.Contains(cidr, "/") {
			ip := net.ParseIP(cidr)
			if ip == nil {
				return nil, fmt.Errorf("web: invalid trusted proxy %q", cidr)
			}
			bits := 128
			if ip.To4() != nil {
				ip = ip.To4()
				bits = 32
			}
			out = append(out, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}

		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("web: invalid trusted proxy %q: %w", cidr, err)
		}
		out = append(out, network)
	}
	return out, nil
}

// TrustedProxies 返回常见的容器与私有网段。
//
// 容器编排（Docker bridge、Kubernetes Pod 网络）里的反向代理通常落在这些网段内，
// 可以直接作为 RealIP 的默认可信列表；暴露在公网时请按实际拓扑收窄。
func TrustedProxies() []*net.IPNet {
	proxies, _ := ParseTrustedProxies(
		"127.0.0.0/8",
		"::1/128",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	)
	return proxies
}

// RealIP 在直连对端属于可信代理时，用 X-Forwarded-For / X-Real-IP 修正客户端地址。
//
// 修正结果写入 Context，后续的 IP()、Logger 与 RateLimit 都会使用真实客户端地址。
// 直连对端不在 trustedProxies 中时忽略这些请求头，防止客户端伪造来源。
//
//	router.Use(web.RealIP(web.TrustedProxies()...))
func RealIP(trustedProxies ...*net.IPNet) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			if ip := resolveClientIP(c.r, trustedProxies); ip != "" {
				c.Set(clientIPKey, ip)
			}
			return next(c)
		}
	}
}

// resolveClientIP 计算真实客户端 IP。
func resolveClientIP(r *http.Request, trustedProxies []*net.IPNet) string {
	remote := remoteIP(r.RemoteAddr)
	if !ipInNetworks(remote, trustedProxies) {
		return remote
	}

	// 从右往左找第一个非可信地址：最右侧是最近一跳，越往左越不可信。
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			ip := net.ParseIP(strings.TrimSpace(parts[i]))
			if ip == nil {
				continue
			}
			if ipInNetworks(ip.String(), trustedProxies) {
				continue
			}
			return ip.String()
		}
	}

	if realIP := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); realIP != nil {
		if !ipInNetworks(realIP.String(), trustedProxies) {
			return realIP.String()
		}
	}
	return remote
}

// remoteIP 从 RemoteAddr 中取出 IP，解析失败时原样返回。
func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// ipInNetworks 判断 ip 是否落在任一网段中，列表为空时返回 false。
func ipInNetworks(ip string, networks []*net.IPNet) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, network := range networks {
		if network != nil && network.Contains(parsed) {
			return true
		}
	}
	return false
}
