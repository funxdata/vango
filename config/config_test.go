package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetWithDefaults(t *testing.T) {
	cfg := New()

	if got := cfg.String("http.addr", ":8080"); got != ":8080" {
		t.Fatalf("String() = %q", got)
	}
	if got := cfg.Int("http.port", 9000); got != 9000 {
		t.Fatalf("Int() = %d", got)
	}
	if got := cfg.Duration("http.timeout", 5*time.Second); got != 5*time.Second {
		t.Fatalf("Duration() = %v", got)
	}
	if cfg.Has("missing") {
		t.Fatal("Has(missing) should be false")
	}
}

func TestSetAndTypedGetters(t *testing.T) {
	cfg := New(WithValues(map[string]any{
		"http": map[string]any{"addr": ":9000", "port": 9000, "debug": "true", "timeout": "30s"},
		"tags": []any{"a", "b"},
	}))

	if got := cfg.String("http.addr", ""); got != ":9000" {
		t.Fatalf("String() = %q", got)
	}
	if got := cfg.Int("http.port", 0); got != 9000 {
		t.Fatalf("Int() = %d", got)
	}
	if !cfg.Bool("http.debug", false) {
		t.Fatal("Bool() should be true")
	}
	if got := cfg.Duration("http.timeout", 0); got != 30*time.Second {
		t.Fatalf("Duration() = %v", got)
	}
	if got := cfg.Strings("tags", nil); len(got) != 2 || got[0] != "a" {
		t.Fatalf("Strings() = %v", got)
	}
	if got := cfg.Strings("http.addr", nil); len(got) != 1 || got[0] != ":9000" {
		t.Fatalf("Strings() from string = %v", got)
	}
}

func TestEnvOverride(t *testing.T) {
	t.Setenv("VANGO_HTTP_ADDR", ":7777")
	cfg := New(WithValues(map[string]any{"http": map[string]any{"addr": ":9000"}}))

	if got := cfg.String("http.addr", ""); got != ":7777" {
		t.Fatalf("env override failed, got %q", got)
	}
}

func TestEnvPrefixDisabled(t *testing.T) {
	t.Setenv("VANGO_HTTP_ADDR", ":7777")
	cfg := New(WithEnvPrefix(""), WithValues(map[string]any{"http": map[string]any{"addr": ":9000"}}))

	if got := cfg.String("http.addr", ""); got != ":9000" {
		t.Fatalf("env should be ignored, got %q", got)
	}
}

func TestLoadFileAndSub(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := []byte(`{"http":{"addr":":7000"},"db":{"dsn":"file:test.db"}}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(WithFile(path))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got := cfg.String("http.addr", ""); got != ":7000" {
		t.Fatalf("String() = %q", got)
	}

	db := cfg.Sub("db")
	if got := db.String("dsn", ""); got != "file:test.db" {
		t.Fatalf("sub String() = %q", got)
	}
	if cfg.File() != path {
		t.Fatalf("File() = %q", cfg.File())
	}
	if db.File() != path {
		t.Fatalf("Sub.File() = %q, want inherited %q", db.File(), path)
	}
}

func TestUnmarshal(t *testing.T) {
	cfg := New(WithValues(map[string]any{
		"http": map[string]any{"addr": ":1234"},
	}))

	var target struct {
		HTTP struct {
			Addr string `json:"addr"`
		} `json:"http"`
	}
	if err := cfg.Unmarshal(&target); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if target.HTTP.Addr != ":1234" {
		t.Fatalf("addr = %q", target.HTTP.Addr)
	}
}

func TestKeys(t *testing.T) {
	cfg := New(WithValues(map[string]any{
		"a": map[string]any{"b": 1},
		"c": 2,
	}))

	keys := cfg.Keys()
	if len(keys) != 2 || keys[0] != "a.b" || keys[1] != "c" {
		t.Fatalf("Keys() = %v", keys)
	}
}

func TestMustStringPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustString should panic on missing key")
		}
	}()
	New().MustString("missing")
}
