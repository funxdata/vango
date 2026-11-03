package web

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/funxdata/vango/log"
)

func TestServerStartAndShutdown(t *testing.T) {
	router := NewRouter()
	router.GET("/ping", func(c *Context) error {
		return c.JSON(http.StatusOK, map[string]string{"pong": "1"})
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	server := NewServer("127.0.0.1:0", router,
		WithListener(listener),
		WithServerLogger(log.Discard()),
	)

	done := make(chan error, 1)
	go func() { done <- server.Start() }()

	url := "http://" + listener.Addr().String() + "/ping"

	var body []byte
	for i := 0; i < 100; i++ {
		response, requestErr := http.Get(url)
		if requestErr == nil {
			body, _ = io.ReadAll(response.Body)
			response.Body.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(body) == 0 {
		t.Fatal("server did not become ready")
	}
	if !strings.Contains(string(body), "pong") {
		t.Fatalf("unexpected body %q", body)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop")
	}
}

func TestServerAddrBeforeStart(t *testing.T) {
	server := NewServer("127.0.0.1:9090", NewRouter())

	if got := server.Addr(); got != "127.0.0.1:9090" {
		t.Fatalf("Addr() = %q", got)
	}
	if server.Handler() == nil {
		t.Fatal("Handler() should not be nil")
	}
}

func TestServerOptions(t *testing.T) {
	server := NewServer(":0", NewRouter(),
		WithTimeouts(time.Second, 2*time.Second, 3*time.Second),
		WithMaxHeaderBytes(1<<20),
		WithTLS("cert.pem", "key.pem"),
	)

	if got := server.Addr(); got != ":0" {
		t.Fatalf("Addr() = %q", got)
	}
}
