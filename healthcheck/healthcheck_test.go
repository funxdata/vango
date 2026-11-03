package healthcheck

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRunSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := Run(server.URL + "/healthz"); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestRunUnhealthyStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if err := Run(server.URL + "/healthz"); err == nil {
		t.Fatal("expected error for non-2xx response")
	}
}

func TestRunUnreachable(t *testing.T) {
	if err := Run("http://127.0.0.1:1/healthz"); err == nil {
		t.Fatal("expected error for unreachable endpoint")
	}
}

func TestDefaultURL(t *testing.T) {
	clearEnv := func(t *testing.T) {
		t.Helper()
		t.Setenv("VANGO_HEALTHCHECK_URL", "")
		t.Setenv("VANGO_HEALTHCHECK_PATH", "")
		t.Setenv("VANGO_HTTP_ADDR", "")
		t.Setenv("PORT", "")
	}

	t.Run("explicit url", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("VANGO_HEALTHCHECK_URL", "http://example.com/healthz")
		if got := DefaultURL(); got != "http://example.com/healthz" {
			t.Fatalf("DefaultURL() = %q", got)
		}
	})

	t.Run("app addr wins over port", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("VANGO_HTTP_ADDR", "0.0.0.0:9090")
		t.Setenv("PORT", "9000")
		if got := DefaultURL(); got != "http://127.0.0.1:9090/healthz" {
			t.Fatalf("DefaultURL() = %q", got)
		}
	})

	t.Run("port env", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("PORT", "9000")
		if got := DefaultURL(); got != "http://127.0.0.1:9000/healthz" {
			t.Fatalf("DefaultURL() = %q", got)
		}
	})

	t.Run("custom path", func(t *testing.T) {
		clearEnv(t)
		t.Setenv("VANGO_HEALTHCHECK_PATH", "readyz")
		if got := DefaultURL(); got != "http://127.0.0.1:8080/readyz" {
			t.Fatalf("DefaultURL() = %q", got)
		}
	})

	t.Run("fallback", func(t *testing.T) {
		clearEnv(t)
		if got := DefaultURL(); got != "http://127.0.0.1:8080/healthz" {
			t.Fatalf("DefaultURL() = %q", got)
		}
	})
}

func TestTimeout(t *testing.T) {
	t.Setenv("VANGO_HEALTHCHECK_TIMEOUT", "")
	if got := Timeout(); got != DefaultTimeout {
		t.Fatalf("Timeout() = %v, want %v", got, DefaultTimeout)
	}

	t.Setenv("VANGO_HEALTHCHECK_TIMEOUT", "1500ms")
	if got := Timeout(); got != 1500*time.Millisecond {
		t.Fatalf("Timeout() = %v", got)
	}

	t.Setenv("VANGO_HEALTHCHECK_TIMEOUT", "invalid")
	if got := Timeout(); got != DefaultTimeout {
		t.Fatalf("Timeout() = %v, want fallback", got)
	}
}
