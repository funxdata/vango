package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouterMatchStaticAndParam(t *testing.T) {
	router := NewRouter()
	router.GET("/users/:id", func(c *Context) error {
		return c.JSON(http.StatusOK, map[string]string{"value": "user:" + c.Param("id")})
	})
	router.GET("/users/me", func(c *Context) error {
		return c.JSON(http.StatusOK, map[string]string{"value": "me"})
	})

	tests := []struct {
		path string
		want string
	}{
		{"/users/42", `"value":"user:42"`},
		{"/users/me", `"value":"me"`},
	}

	for _, tc := range tests {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", tc.path, recorder.Code)
		}
		if !strings.Contains(recorder.Body.String(), tc.want) {
			t.Fatalf("%s: body = %q, want to contain %q", tc.path, recorder.Body.String(), tc.want)
		}
	}
}

func TestRouterWildcard(t *testing.T) {
	router := NewRouter()
	router.GET("/files/*filepath", func(c *Context) error {
		return c.JSON(http.StatusOK, map[string]string{"path": c.Param("filepath")})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/files/a/b/c.txt", nil))

	if got := recorder.Body.String(); !strings.Contains(got, `"path":"a/b/c.txt"`) {
		t.Fatalf("wildcard body = %q", got)
	}
}

func TestRouterNotFoundAndMethodNotAllowed(t *testing.T) {
	router := NewRouter()
	router.GET("/only-get", func(c *Context) error {
		return c.JSON(http.StatusOK, map[string]string{"ok": "true"})
	})

	notFound := httptest.NewRecorder()
	router.ServeHTTP(notFound, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", notFound.Code)
	}
	if !strings.Contains(notFound.Body.String(), "route not found") {
		t.Fatalf("unexpected body %q", notFound.Body.String())
	}

	notAllowed := httptest.NewRecorder()
	router.ServeHTTP(notAllowed, httptest.NewRequest(http.MethodPost, "/only-get", nil))
	if notAllowed.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", notAllowed.Code)
	}
}

func TestGroupMiddlewareAndPrefix(t *testing.T) {
	router := NewRouter()

	var order []string
	router.Use(func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			order = append(order, "global")
			return next(c)
		}
	})

	group := router.Group("/api", func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			order = append(order, "group")
			return next(c)
		}
	})
	group.GET("/ping", func(c *Context) error {
		order = append(order, "handler")
		return c.JSON(http.StatusOK, map[string]string{"ping": "pong"})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/ping", nil))

	if !strings.Contains(recorder.Body.String(), `"ping":"pong"`) {
		t.Fatalf("body = %q", recorder.Body.String())
	}
	if strings.Join(order, ",") != "global,group,handler" {
		t.Fatalf("middleware order = %v", order)
	}
}

func TestRouterHeadFallsBackToGet(t *testing.T) {
	router := NewRouter()
	router.GET("/head", func(c *Context) error {
		return c.JSON(http.StatusOK, map[string]string{"ok": "true"})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodHead, "/head", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

func TestApplyOrder(t *testing.T) {
	var order []string
	mw := func(name string) Middleware {
		return func(next HandlerFunc) HandlerFunc {
			return func(c *Context) error {
				order = append(order, name)
				return next(c)
			}
		}
	}

	handler := Chain(mw("a"), mw("b"))(func(c *Context) error { return nil })
	_ = handler(nil)
	if strings.Join(order, "") != "ab" {
		t.Fatalf("order = %v, want a before b", order)
	}
}
