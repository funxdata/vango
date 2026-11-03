package web

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// Server 封装 http.Server，提供优雅启停。
type Server struct {
	srv    *http.Server
	logger *slog.Logger

	mu       sync.RWMutex
	listener net.Listener

	certFile string
	keyFile  string
}

// ServerOption 定制 Server。
type ServerOption func(*Server)

// WithServerLogger 设置服务日志器。
func WithServerLogger(logger *slog.Logger) ServerOption {
	return func(s *Server) { s.logger = logger }
}

// WithTLS 开启 HTTPS。
func WithTLS(certFile, keyFile string) ServerOption {
	return func(s *Server) {
		s.certFile = certFile
		s.keyFile = keyFile
	}
}

// WithTimeouts 设置读写与空闲超时。
func WithTimeouts(read, write, idle time.Duration) ServerOption {
	return func(s *Server) {
		s.srv.ReadTimeout = read
		s.srv.WriteTimeout = write
		s.srv.IdleTimeout = idle
	}
}

// WithMaxHeaderBytes 设置请求头大小上限。
func WithMaxHeaderBytes(n int) ServerOption {
	return func(s *Server) { s.srv.MaxHeaderBytes = n }
}

// WithListener 使用自定义 listener，常用于测试或接入已有的端口管理。
func WithListener(listener net.Listener) ServerOption {
	return func(s *Server) { s.listener = listener }
}

// NewServer 创建 HTTP 服务。
//
// 默认只设置 ReadHeaderTimeout 以防慢速攻击，读写超时按需通过 WithTimeouts 开启。
func NewServer(addr string, handler http.Handler, opts ...ServerOption) *Server {
	s := &Server{
		srv: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.logger == nil {
		s.logger = slog.Default()
	}
	return s
}

// Start 阻塞地启动服务，正常关闭时返回 nil。
func (s *Server) Start() error {
	s.mu.Lock()
	listener := s.listener
	if listener == nil {
		ln, err := net.Listen("tcp", s.srv.Addr)
		if err != nil {
			s.mu.Unlock()
			return err
		}
		listener = ln
		s.listener = ln
	}
	s.mu.Unlock()

	s.logger.Info("http server listening", "addr", listener.Addr().String())

	var err error
	if s.certFile != "" && s.keyFile != "" {
		err = s.srv.ServeTLS(listener, s.certFile, s.keyFile)
	} else {
		err = s.srv.Serve(listener)
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown 优雅关闭服务，ctx 控制等待上限。
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

// Close 立即关闭服务。
func (s *Server) Close() error {
	return s.srv.Close()
}

// Addr 返回实际监听地址，使用随机端口测试时可据此拿到端口。
func (s *Server) Addr() string {
	s.mu.RLock()
	listener := s.listener
	s.mu.RUnlock()
	if listener != nil {
		return listener.Addr().String()
	}
	return s.srv.Addr
}

// Handler 返回底层 http.Handler。
func (s *Server) Handler() http.Handler {
	return s.srv.Handler
}
