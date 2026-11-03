// Package httpx 是带默认配置的 HTTP 客户端封装。
//
// 它只依赖标准库，提供 JSON、表单、文件上传与代理等常用能力；
// 需要更细粒度控制时，直接使用内嵌的 *http.Client。
//
//	cli := httpx.New(httpx.WithTimeout(5 * time.Second))
//	var user User
//	if err := cli.GetJSON("https://api.example.com/users/1", &user); err != nil { ... }
package httpx

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Client 是 vango 的 HTTP 客户端。
type Client struct {
	*http.Client
}

// Option 定制 Client。
type Option func(*Client)

// WithTimeout 设置整体超时，默认 30s。
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) { c.Timeout = timeout }
}

// WithProxy 通过代理发送请求。
func WithProxy(proxyURL *url.URL) Option {
	return func(c *Client) {
		transport, ok := c.Transport.(*http.Transport)
		if !ok || transport == nil {
			transport = http.DefaultTransport.(*http.Transport).Clone()
			c.Transport = transport
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
}

// WithInsecureSkipVerify 关闭 TLS 证书校验，仅建议在测试环境使用。
func WithInsecureSkipVerify() Option {
	return func(c *Client) {
		transport, ok := c.Transport.(*http.Transport)
		if !ok || transport == nil {
			transport = http.DefaultTransport.(*http.Transport).Clone()
			c.Transport = transport
		}
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		}
		transport.TLSClientConfig.InsecureSkipVerify = true
	}
}

// New 创建一个客户端，默认超时 30s。
func New(opts ...Option) *Client {
	c := &Client{Client: &http.Client{Timeout: 30 * time.Second}}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// NewProxyClient 创建一个走指定代理的客户端。
func NewProxyClient(proxyURL *url.URL) *Client {
	return New(WithProxy(proxyURL))
}

// GetBytes 发起 GET 请求并返回响应体，状态码 >= 400 时返回错误。
func (c *Client) GetBytes(uri string) ([]byte, error) {
	return c.doBytes(http.MethodGet, uri, "", nil)
}

// GetJSON 发起 GET 请求并把 JSON 响应解码到 dst。
func (c *Client) GetJSON(uri string, dst any) error {
	return c.doJSON(http.MethodGet, uri, nil, dst)
}

// PostJSON 把 payload 序列化为 JSON 并 POST，返回原始响应。
func (c *Client) PostJSON(uri string, payload any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("httpx: encode payload: %w", err)
	}
	return c.Post(uri, "application/json", bytes.NewReader(body))
}

// DoJSON 发送 req 并把 JSON 响应解码到 dst。
func (c *Client) DoJSON(req *http.Request, dst any) error {
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("httpx: %s %s returned %d", req.Method, req.URL.String(), resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

// PostForm 以 application/x-www-form-urlencoded 提交表单并返回响应体。
func (c *Client) PostForm(uri string, values url.Values) ([]byte, error) {
	return c.doBytes(http.MethodPost, uri, "application/x-www-form-urlencoded", bytes.NewBufferString(values.Encode()))
}

// PostFile 以 multipart 形式上传一个已打开的文件。
func (c *Client) PostFile(uri, formName string, file *os.File) (*http.Response, error) {
	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)

	part, err := writer.CreateFormFile(formName, file.Name())
	if err != nil {
		return nil, fmt.Errorf("httpx: create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("httpx: copy file: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("httpx: close multipart writer: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, uri, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return c.Do(req)
}

// IsExists 判断 URL 是否可访问（状态码小于 400）。
func (c *Client) IsExists(uri string) bool {
	resp, err := c.Get(uri)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < http.StatusBadRequest
}

func (c *Client) doJSON(method, uri string, body io.Reader, dst any) error {
	req, err := http.NewRequest(method, uri, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.DoJSON(req, dst)
}

func (c *Client) doBytes(method, uri, contentType string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, uri, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("httpx: %s %s returned %d", method, uri, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// defaultClient 是包级函数的默认客户端。
var defaultClient = New()

// Get 使用默认客户端发起 GET 请求。
func Get(uri string) ([]byte, error) { return defaultClient.GetBytes(uri) }

// Post 使用默认客户端以指定 Content-Type 发起 POST 请求。
func Post(uri, contentType string, body []byte) ([]byte, error) {
	return defaultClient.doBytes(http.MethodPost, uri, contentType, bytes.NewReader(body))
}
