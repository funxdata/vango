package version

import (
	"bytes"
	"strings"
	"testing"
)

func TestComplete(t *testing.T) {
	originalVersion, originalCommit, originalBuildAt := Version, GitCommit, BuildAt
	t.Cleanup(func() {
		Version, GitCommit, BuildAt = originalVersion, originalCommit, originalBuildAt
	})

	Version, GitCommit, BuildAt = "v1.2.3", "abc123", "2026-01-02T03:04:05Z"
	if got, want := Complete(), "v1.2.3 abc123 2026-01-02T03:04:05Z"; got != want {
		t.Fatalf("Complete() = %q, want %q", got, want)
	}
	if GetVersion() != "v1.2.3" {
		t.Fatalf("GetVersion() = %q", GetVersion())
	}
	if GetCompleteVersion() != Complete() {
		t.Fatalf("GetCompleteVersion() should equal Complete()")
	}
}

func TestPrint(t *testing.T) {
	Version, GitCommit, BuildAt = "v0.1.0", "deadbeef", "2026-01-02T03:04:05Z"

	var buf bytes.Buffer
	Print(&buf)

	for _, want := range []string{"v0.1.0", "deadbeef", "2026-01-02T03:04:05Z"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("Print output %q does not contain %q", buf.String(), want)
		}
	}
}
