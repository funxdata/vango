package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseTrustedProxies(t *testing.T) {
	proxies, err := ParseTrustedProxies("10.0.0.0/8", "192.0.2.1", "::1")
	if err != nil {
		t.Fatalf("ParseTrustedProxies returned error: %v", err)
	}
	if len(proxies) != 3 {
		t.Fatalf("proxies = %d, want 3", len(proxies))
	}
	if _, err := ParseTrustedProxies("not-an-ip"); err == nil {
		t.Fatal("expected error for invalid proxy")
	}
}

func TestClientIPIgnoresHeadersFromUntrustedPeer(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/ip", nil)
	request.RemoteAddr = "192.0.2.9:4321"
	request.Header.Set("X-Forwarded-For", "203.0.113.7")
	request.Header.Set("X-Real-IP", "203.0.113.8")

	proxies, err := ParseTrustedProxies("10.0.0.0/8")
	if err != nil {
		t.Fatalf("ParseTrustedProxies returned error: %v", err)
	}

	c := newContext(httptest.NewRecorder(), request)
	if got := c.ClientIP(proxies...); got != "192.0.2.9" {
		t.Fatalf("ClientIP() = %q, want 192.0.2.9", got)
	}
}

func TestClientIPHonorsForwardedChain(t *testing.T) {
	proxies, err := ParseTrustedProxies("10.0.0.0/8")
	if err != nil {
		t.Fatalf("ParseTrustedProxies returned error: %v", err)
	}

	tests := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{
			name:    "first untrusted from the right",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.4, 203.0.113.7, 10.0.0.5"},
			want:    "203.0.113.7",
		},
		{
			name:    "all proxies fall back to real ip header",
			headers: map[string]string{"X-Forwarded-For": "10.0.0.6, 10.0.0.5", "X-Real-IP": "203.0.113.9"},
			want:    "203.0.113.9",
		},
		{
			name:    "no headers",
			headers: nil,
			want:    "10.0.0.5",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/ip", nil)
			request.RemoteAddr = "10.0.0.5:1234"
			for key, value := range tc.headers {
				request.Header.Set(key, value)
			}

			c := newContext(httptest.NewRecorder(), request)
			if got := c.ClientIP(proxies...); got != tc.want {
				t.Fatalf("ClientIP() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRealIPMiddlewareOverridesContextIP(t *testing.T) {
	router := NewRouter()
	router.Use(RealIP(TrustedProxies()...))
	router.GET("/ip", func(c *Context) error {
		return OK(c, map[string]string{"ip": c.IP()})
	})

	request := httptest.NewRequest(http.MethodGet, "/ip", nil)
	request.RemoteAddr = "172.17.0.2:5555"
	request.Header.Set("X-Forwarded-For", "203.0.113.7")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if body := recorder.Body.String(); !strings.Contains(body, "203.0.113.7") {
		t.Fatalf("body = %q, want real client ip", body)
	}
}

func TestRealIPMiddlewareKeepsUntrustedRemoteAddr(t *testing.T) {
	router := NewRouter()
	router.Use(RealIP(TrustedProxies()...))
	router.GET("/ip", func(c *Context) error {
		return OK(c, map[string]string{"ip": c.IP()})
	})

	request := httptest.NewRequest(http.MethodGet, "/ip", nil)
	request.RemoteAddr = "198.51.100.20:5555"
	request.Header.Set("X-Forwarded-For", "203.0.113.7")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if body := recorder.Body.String(); !strings.Contains(body, "198.51.100.20") {
		t.Fatalf("body = %q, want remote addr", body)
	}
}
