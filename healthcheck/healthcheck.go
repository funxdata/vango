// Package healthcheck 提供容器健康探针的小工具。
//
// 镜像通常不含 curl，因此可以把同一个二进制当作探针：
//
//	func main() {
//		if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
//			if err := healthcheck.Run(""); err != nil {
//				fmt.Fprintln(os.Stderr, err)
//				os.Exit(1)
//			}
//			return
//		}
//		// 正常启动应用...
//	}
//
// Dockerfile 中配置：
//
//	HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
//	  CMD ["/app", "healthcheck"]
//
// 探针地址与超时都可以通过环境变量调整，便于容器与编排系统复用同一镜像：
//
//	VANGO_HEALTHCHECK_URL       完整地址，优先级最高
//	VANGO_HEALTHCHECK_PATH      探针路径，默认 /healthz（可用 /readyz）
//	VANGO_HEALTHCHECK_TIMEOUT   请求超时，默认 3s
//	VANGO_HTTP_ADDR             应用监听地址，探针从中取端口
//	PORT                        监听端口（兼容托管平台的动态端口）
package healthcheck

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// DefaultTimeout 是探针的默认请求超时。
	DefaultTimeout = 3 * time.Second
	// DefaultPath 是探针的默认请求路径。
	DefaultPath = "/healthz"
	// DefaultPort 是找不到端口配置时使用的端口。
	DefaultPort = "8080"
)

// DefaultURL 返回探针地址。
//
// 优先级：VANGO_HEALTHCHECK_URL > VANGO_HTTP_ADDR > PORT > 8080，
// 路径来自 VANGO_HEALTHCHECK_PATH，默认 /healthz。
func DefaultURL() string {
	if url := strings.TrimSpace(os.Getenv("VANGO_HEALTHCHECK_URL")); url != "" {
		return url
	}
	return "http://" + net.JoinHostPort("127.0.0.1", ProbePort()) + ProbePath()
}

// ProbePort 返回探针使用的端口。
//
// 优先使用应用真实的监听地址 VANGO_HTTP_ADDR（例如 ":8080"、"0.0.0.0:9090"），
// 这样在容器里改了端口，HEALTHCHECK 也会跟着走。
func ProbePort() string {
	if port := portFromAddr(os.Getenv("VANGO_HTTP_ADDR")); port != "" {
		return port
	}
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		return port
	}
	return DefaultPort
}

// ProbePath 返回探针的请求路径，始终以 / 开头。
func ProbePath() string {
	path := strings.TrimSpace(os.Getenv("VANGO_HEALTHCHECK_PATH"))
	if path == "" {
		return DefaultPath
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

// Timeout 返回探针的请求超时，可由 VANGO_HEALTHCHECK_TIMEOUT 覆盖（如 "1s"）。
func Timeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("VANGO_HEALTHCHECK_TIMEOUT"))
	if raw == "" {
		return DefaultTimeout
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		return DefaultTimeout
	}
	return timeout
}

// Run 请求健康检查地址，返回非 2xx 或请求失败都视为不健康。
//
// url 为空时使用 DefaultURL。探针总是直连本机，不受 HTTP_PROXY 影响。
func Run(url string) error {
	if url == "" {
		url = DefaultURL()
	}

	client := &http.Client{Timeout: Timeout(), Transport: directTransport()}
	response, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("healthcheck: GET %s: %w", url, err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("healthcheck: GET %s returned %d", url, response.StatusCode)
	}
	return nil
}

// directTransport 返回关闭代理的 Transport，避免容器内的 HTTP_PROXY 影响本机探针。
func directTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return transport
}

// portFromAddr 从监听地址中取出端口，无法解析时返回空字符串。
func portFromAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if _, port, err := net.SplitHostPort(addr); err == nil {
		return port
	}
	// 允许只写端口号，例如 VANGO_HTTP_ADDR=9090。
	if !strings.Contains(addr, ":") {
		return addr
	}
	return ""
}
