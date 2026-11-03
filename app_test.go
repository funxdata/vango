package vango

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/funxdata/vango/log"
	"github.com/funxdata/vango/web"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	return New(
		WithName("test"),
		WithAddress("127.0.0.1:0"),
		WithLogger(log.Discard()),
	)
}

func TestHealthEndpoints(t *testing.T) {
	app := newTestApp(t)

	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("/healthz status = %d", recorder.Code)
	}

	var payload struct {
		Code int               `json:"code"`
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode /healthz: %v", err)
	}
	if payload.Code != 0 || payload.Data["status"] != "ok" {
		t.Fatalf("unexpected /healthz payload: %s", recorder.Body.String())
	}

	ready := httptest.NewRecorder()
	app.Handler().ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("/readyz status = %d", ready.Code)
	}
}

func TestHealthEndpointsCanBeDisabled(t *testing.T) {
	app := New(WithLogger(log.Discard()), WithoutHealthEndpoints())

	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func TestRouteRegistration(t *testing.T) {
	app := newTestApp(t)
	app.GET("/api/v1/echo/:value", func(c *web.Context) error {
		return web.OK(c, c.Param("value"))
	})

	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/echo/vango", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"data":"vango"`) {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestModuleRegistration(t *testing.T) {
	app := newTestApp(t)

	module := NewModule("demo", func(a *App) error {
		a.GET("/module", func(c *web.Context) error { return web.OK(c, "ok") })
		return nil
	})
	if err := app.Register(module); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/module", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

func TestStartStopAndHooks(t *testing.T) {
	app := newTestApp(t)

	var events []string
	app.OnStart("first", func(ctx context.Context) error {
		events = append(events, "start:first")
		return nil
	})
	app.OnStart("second", func(ctx context.Context) error {
		events = append(events, "start:second")
		return nil
	})
	app.OnStop("first", func(ctx context.Context) error {
		events = append(events, "stop:first")
		return nil
	})
	app.OnStop("second", func(ctx context.Context) error {
		events = append(events, "stop:second")
		return nil
	})

	if err := app.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if addr := app.Addr(); addr == "" || addr == "127.0.0.1:0" {
		t.Fatalf("unexpected address %q", addr)
	}

	if err := app.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}

	want := []string{"start:first", "start:second", "stop:second", "stop:first"}
	if len(events) != len(want) {
		t.Fatalf("hook events = %v, want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("hook events = %v, want %v", events, want)
		}
	}
}

func TestStartTwiceFails(t *testing.T) {
	app := newTestApp(t)
	if err := app.Start(context.Background()); err != nil {
		t.Fatalf("first Start returned error: %v", err)
	}
	t.Cleanup(func() { _ = app.Stop() })

	if err := app.Start(context.Background()); err == nil {
		t.Fatal("second Start should fail")
	}
}

func TestStartHookFailureAllowsRestart(t *testing.T) {
	app := New(WithAddress("127.0.0.1:0"), WithLogger(log.Discard()))

	app.OnStart("fail", func(ctx context.Context) error {
		return errors.New("boom")
	})
	if err := app.Start(context.Background()); err == nil {
		t.Fatal("Start should fail when a start hook fails")
	}

	app.startHooks = nil
	if err := app.Start(context.Background()); err != nil {
		t.Fatalf("restart after failed hook returned error: %v", err)
	}
	t.Cleanup(func() { _ = app.Stop() })
}
