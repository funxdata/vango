package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffoldCreatesProject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo")

	err := Scaffold(dir, Options{
		Name:         "demo",
		ModulePath:   "example.com/demo",
		VangoVersion: "v0.1.0",
	})
	if err != nil {
		t.Fatalf("Scaffold returned error: %v", err)
	}

	for _, name := range []string{
		"go.mod", "main.go", "config.json", "Dockerfile",
		".dockerignore", "docker-compose.yml", "README.md", ".gitignore",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}

	goMod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if !strings.Contains(string(goMod), "module example.com/demo") {
		t.Fatalf("unexpected go.mod: %s", goMod)
	}

	dockerfile, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	for _, want := range []string{"HEALTHCHECK", "distroless", "healthcheck"} {
		if !strings.Contains(string(dockerfile), want) {
			t.Fatalf("Dockerfile missing %q:\n%s", want, dockerfile)
		}
	}

	mainGo, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if !strings.Contains(string(mainGo), "healthcheck.Run") {
		t.Fatalf("main.go missing healthcheck wiring:\n%s", mainGo)
	}
}

func TestScaffoldRejectsInvalidName(t *testing.T) {
	if err := Scaffold(filepath.Join(t.TempDir(), "x"), Options{Name: "../escape"}); err == nil {
		t.Fatal("expected error for invalid project name")
	}
}

func TestScaffoldRejectsNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if err := Scaffold(dir, Options{Name: "demo"}); err == nil {
		t.Fatal("expected error for non-empty directory")
	}
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := Run([]string{"version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "vango") {
		t.Fatalf("unexpected output %q", stdout.String())
	}
}

func TestRunHelp(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := Run(nil, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stdout.String(), "new <name>") {
		t.Fatalf("unexpected usage output %q", stdout.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := Run([]string{"nope"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestRunNew(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo")

	var stdout, stderr strings.Builder
	code := Run([]string{"new", "-dir", dir, "-module", "example.com/demo", "demo"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "main.go")); err != nil {
		t.Fatalf("main.go not created: %v", err)
	}
}
