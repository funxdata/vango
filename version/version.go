// Package version 提供构建期通过 -ldflags 注入的版本信息。
//
//	go build -ldflags "-X github.com/funxdata/vango/version.Version=v1.0.0 \
//	  -X github.com/funxdata/vango/version.GitCommit=$(git rev-parse --short HEAD) \
//	  -X github.com/funxdata/vango/version.BuildAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
package version

import (
	"fmt"
	"io"
	"os"
	"strings"
)

var (
	// Version 是语义化版本号，默认 dev。
	Version = "dev"
	// GitCommit 是构建时的提交号。
	GitCommit = ""
	// BuildAt 是构建时间。
	BuildAt = ""
)

// Get 返回版本号。
func Get() string { return Version }

// GetVersion 是 Get 的兼容别名。
func GetVersion() string { return Version }

// GetGitCommit 返回构建时的提交号。
func GetGitCommit() string { return GitCommit }

// GetBuildAt 返回构建时间字符串。
func GetBuildAt() string { return BuildAt }

// GetBuildAtString 是 GetBuildAt 的兼容别名。
func GetBuildAtString() string { return BuildAt }

// Complete 返回包含提交号的完整版本描述。
func Complete() string {
	parts := []string{Version}
	if GitCommit != "" {
		parts = append(parts, GitCommit)
	}
	if BuildAt != "" {
		parts = append(parts, BuildAt)
	}
	return strings.Join(parts, " ")
}

// GetCompleteVersion 是 Complete 的兼容别名。
func GetCompleteVersion() string { return Complete() }

// String 返回完整版本描述。
func String() string { return Complete() }

// Print 把版本信息写入 w，w 为 nil 时写入标准输出。
func Print(w io.Writer) {
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprintf(w, "Version:    %s\n", Version)
	fmt.Fprintf(w, "Git Commit: %s\n", GitCommit)
	fmt.Fprintf(w, "Build At:   %s\n", BuildAt)
}
