package web

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/funxdata/vango/errs"
)

func TestLoggerRecordsFinalStatus(t *testing.T) {
	tests := []struct {
		name       string
		handler    HandlerFunc
		wantStatus int
	}{
		{
			name: "success",
			handler: func(c *Context) error {
				return OK(c, map[string]string{"ok": "true"})
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "structured error",
			handler: func(c *Context) error {
				return errs.NotFound("missing")
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "plain error",
			handler: func(c *Context) error {
				return errs.Internal("boom")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, nil))

			router := NewRouter()
			router.Use(Logger(logger))
			router.GET("/log", tc.handler)

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/log", nil))
			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.wantStatus)
			}

			var entry map[string]any
			if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
				t.Fatalf("decode log entry %q: %v", buf.String(), err)
			}
			if got := entry["status"]; got != float64(tc.wantStatus) {
				t.Fatalf("logged status = %v, want %d", got, tc.wantStatus)
			}
		})
	}
}

func TestRequestIDFromHeader(t *testing.T) {
	router := NewRouter()
	router.Use(RequestID())
	router.GET("/id", func(c *Context) error { return OK(c, c.RequestID()) })

	request := httptest.NewRequest(http.MethodGet, "/id", nil)
	request.Header.Set(HeaderRequestID, "given-id")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if got := recorder.Header().Get(HeaderRequestID); got != "given-id" {
		t.Fatalf("response header = %q", got)
	}
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	router := NewRouter()
	router.Use(CORS(CORSOptions{AllowOrigins: []string{"https://example.com"}}))
	router.GET("/cors", func(c *Context) error { return OK(c, nil) })

	request := httptest.NewRequest(http.MethodGet, "/cors", nil)
	request.Header.Set("Origin", "https://example.com")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("allow origin = %q", got)
	}
}

func TestRateLimitRejectsBurst(t *testing.T) {
	router := NewRouter()
	router.Use(RateLimit(1, 1))
	router.GET("/limited", func(c *Context) error { return OK(c, nil) })

	send := func() int {
		request := httptest.NewRequest(http.MethodGet, "/limited", nil)
		request.RemoteAddr = "192.0.2.1:1234"
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder.Code
	}

	if code := send(); code != http.StatusOK {
		t.Fatalf("first request status = %d", code)
	}
	if code := send(); code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want 429", code)
	}
}

func TestLoggerSkipsProbePaths(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	router := NewRouter()
	router.Use(LoggerWithOptions(LoggerOptions{
		Logger:    logger,
		SkipPaths: []string{"/healthz"},
	}))
	router.GET("/healthz", func(c *Context) error { return OK(c, nil) })
	router.GET("/api", func(c *Context) error { return OK(c, nil) })

	for _, path := range []string{"/healthz", "/api"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("log lines = %d (%q), want 1", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], `"path":"/api"`) {
		t.Fatalf("unexpected log line %q", lines[0])
	}
}

func TestSecureHeaders(t *testing.T) {
	router := NewRouter()
	router.Use(SecureHeaders())
	router.GET("/", func(c *Context) error { return OK(c, nil) })

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := recorder.Header().Get(header); got != want {
			t.Fatalf("%s = %q, want %q", header, got, want)
		}
	}
}
