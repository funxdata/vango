package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetBytesRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", 64)))
	}))
	defer server.Close()

	client := New(WithMaxResponseBytes(16))
	if _, err := client.GetBytes(server.URL); err == nil {
		t.Fatal("expected error for response larger than limit")
	}

	client = New(WithMaxResponseBytes(1024))
	if _, err := client.GetBytes(server.URL); err != nil {
		t.Fatalf("GetBytes returned error: %v", err)
	}
}

func TestErrorOmitsQueryString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := Get(server.URL + "/private?token=topsecret")
	if err == nil {
		t.Fatal("expected error for non-2xx response")
	}
	if strings.Contains(err.Error(), "topsecret") {
		t.Fatalf("error message leaks query string: %v", err)
	}
}
