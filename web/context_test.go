package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type createUser struct {
	Name  string   `json:"name"`
	Age   int      `json:"age"`
	Roles []string `json:"roles"`
}

func TestBindJSON(t *testing.T) {
	router := NewRouter()
	router.POST("/users", func(c *Context) error {
		var payload createUser
		if err := c.Bind(&payload); err != nil {
			return err
		}
		return OK(c, payload)
	})

	request := httptest.NewRequest(http.MethodPost, "/users",
		strings.NewReader(`{"name":"vango","age":3,"roles":["admin","dev"]}`))
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"name":"vango"`) {
		t.Fatalf("unexpected body %s", recorder.Body.String())
	}
}

func TestBindQuery(t *testing.T) {
	router := NewRouter()
	router.GET("/query", func(c *Context) error {
		var payload createUser
		if err := c.BindQuery(&payload); err != nil {
			return err
		}
		return OK(c, map[string]any{"name": payload.Name, "age": payload.Age, "roles": payload.Roles})
	})

	queryRecorder := httptest.NewRecorder()
	router.ServeHTTP(queryRecorder, httptest.NewRequest(http.MethodGet, "/query?name=query&age=7&roles=a,b", nil))
	if !strings.Contains(queryRecorder.Body.String(), `"name":"query"`) {
		t.Fatalf("query bind failed: %s", queryRecorder.Body.String())
	}
	if !strings.Contains(queryRecorder.Body.String(), `"age":7`) {
		t.Fatalf("query int bind failed: %s", queryRecorder.Body.String())
	}
}

func TestHandlerErrorIsMappedToResponse(t *testing.T) {
	router := NewRouter()
	router.GET("/boom", func(c *Context) error {
		return badRequest("name is required")
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"message":"name is required"`) {
		t.Fatalf("unexpected body %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"error_code":"BAD_REQUEST"`) {
		t.Fatalf("missing stable error code in body %s", recorder.Body.String())
	}
}

func TestParamsAndHelpers(t *testing.T) {
	params := Params{{Key: "id", Value: "42"}, {Key: "name", Value: "vango"}}
	if params.Get("id") != "42" {
		t.Fatalf("Get(id) = %q", params.Get("id"))
	}
	if params.Get("missing") != "" {
		t.Fatalf("Get(missing) should be empty")
	}
	if !params.Has("name") {
		t.Fatal("Has(name) should be true")
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	router := NewRouter()
	router.Use(RequestID())
	router.GET("/id", func(c *Context) error {
		return c.JSON(http.StatusOK, map[string]string{"id": c.RequestID()})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/id", nil))

	header := recorder.Header().Get(HeaderRequestID)
	if header == "" {
		t.Fatal("response should carry a request id")
	}
	if !strings.Contains(recorder.Body.String(), header) {
		t.Fatalf("body %q does not contain header %q", recorder.Body.String(), header)
	}
}

func TestRecoverMiddleware(t *testing.T) {
	router := NewRouter()
	router.Use(Recover(nil))
	router.GET("/panic", func(c *Context) error {
		panic("boom")
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
}
